package game

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// ---------------------------------------------------------------------------
// Tuning constants.
//
// These are calibrated in race_test.go by Monte Carlo. The headline target is
// that a build with a 5% rating advantage — a realistic gap between ladder
// neighbours — wins about 65% of the time. Change one of these and run
// `go test ./internal/game -run Calibration -v` to see what it did.
// ---------------------------------------------------------------------------
const (
	// CarWeight/DriverWeight split how much the machine matters vs the human.
	CarWeight    = 0.72
	DriverWeight = 0.28

	// TimeExponent compresses raw performance into lap time. A 10% better car
	// is not a 10% faster run down a mountain.
	TimeExponent = 0.55

	// FormSigma is the correlated "how this run is going" term. It is the
	// dominant source of upsets — a 65% favourite by definition means the
	// underdog takes it better than one time in three, and that has to come
	// from somewhere.
	FormSigma = 0.047

	// FormMomentum makes form drift over a run rather than being fixed at the
	// start line: a driver finds a rhythm, or loses one. Purely a drama knob —
	// the sequence is rescaled so total variance (and the 65% calibration) is
	// unchanged by it. At 1.0 the race is decided before turn one; lower values
	// buy lead changes and comebacks.
	FormMomentum = 0.82

	// SegmentSigma is per-corner noise, scaled by segment risk.
	SegmentSigma = 0.030

	// Mistakes: lockups, missed apexes, a wheel in the dirt.
	MistakeBaseProb = 0.09
	MistakeCostMin  = 0.08
	MistakeCostMax  = 0.26

	// TurboLagPenalty converts a big turbo's lag into extra variance.
	TurboLagPenalty = 1.0
)

// ---------------------------------------------------------------------------
// Performance model
// ---------------------------------------------------------------------------

// segmentPerf scores a build through one segment. A stock car with an average
// driver returns ~100. Performance is linear in every input, which is what
// makes the rating scale meaningful and the calibration test possible.
func segmentPerf(b Build, seg Segment, progress float64) float64 {
	w, ok := segmentWeights[seg.Kind]
	if !ok {
		w = segmentWeights[KindTechnical]
	}

	car := w.Power*(b.Car.PowerToWt/ReferencePowerToWt) +
		w.Grip*(b.Car.Grip/ReferenceGrip) +
		w.Handling*(b.Car.Handling/ReferenceHandling) +
		w.Braking*(b.Car.Braking/ReferenceBraking)

	drv := w.Bravery*(b.Driver.Bravery/ReferenceDriverStat) +
		w.Intelligence*(b.Driver.Intelligence/ReferenceDriverStat) +
		w.Reflexes*(b.Driver.Reflexes/ReferenceDriverStat) +
		w.Consistency*(b.Driver.Consistency/ReferenceDriverStat) +
		w.Stamina*(b.Driver.Stamina/ReferenceDriverStat)

	perf := 100 * (CarWeight*car + DriverWeight*drv)
	perf *= drivetrainMod(seg.Kind, b.Car.Drivetrain)
	perf *= setupMod(b, seg, progress)
	perf *= staminaFactor(b.Driver.Stamina, progress)

	if perf < 1 {
		perf = 1
	}
	return perf
}

// staminaFactor bleeds performance in the back third of the run for drivers
// who cannot hold concentration. Above-average stamina earns a small bonus.
func staminaFactor(stamina, progress float64) float64 {
	if progress <= 0.6 {
		return 1
	}
	deficit := (ReferenceDriverStat - stamina) / ReferenceDriverStat
	if deficit > 1 {
		deficit = 1
	}
	if deficit < -0.5 {
		deficit = -0.5
	}
	return 1 - (progress-0.6)/0.4*deficit*0.06
}

// TrackRating is the average performance a build carries over a track,
// weighted by how much time each segment takes.
func TrackRating(b Build, t TrackDef) float64 {
	var num, den float64
	for i, seg := range t.Segments {
		progress := float64(i) / math.Max(1, float64(len(t.Segments)-1))
		num += segmentPerf(b, seg, progress) * seg.BaseSeconds
		den += seg.BaseSeconds
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// OverallRating averages a build across every track in the game. This is the
// number shown in the garage and used to seed ladder expectations.
func OverallRating(b Build) float64 {
	var sum float64
	for _, t := range Tracks {
		sum += TrackRating(b, t)
	}
	return sum / float64(len(Tracks))
}

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

type SegmentResult struct {
	Index      int         `json:"index"`
	Name       string      `json:"name"`
	Kind       SegmentKind `json:"kind"`
	ChallTime  float64     `json:"challenger_time"`
	OppTime    float64     `json:"opponent_time"`
	ChallTotal float64     `json:"challenger_total"`
	OppTotal   float64     `json:"opponent_total"`
	// Gap is positive when the challenger is ahead.
	Gap          float64 `json:"gap"`
	ChallMistake bool    `json:"challenger_mistake"`
	OppMistake   bool    `json:"opponent_mistake"`
	Note         string  `json:"note,omitempty"`
}

type RaceResult struct {
	TrackID     string          `json:"track_id"`
	TrackName   string          `json:"track_name"`
	Seed        uint64          `json:"seed"`
	Segments    []SegmentResult `json:"segments"`
	ChallTotal  float64         `json:"challenger_total"`
	OppTotal    float64         `json:"opponent_total"`
	ChallWon    bool            `json:"challenger_won"`
	Margin      float64         `json:"margin"`
	ChallRating float64         `json:"challenger_rating"`
	OppRating   float64         `json:"opponent_rating"`
	LeadChanges int             `json:"lead_changes"`
	Upset       bool            `json:"upset"`
}

// ---------------------------------------------------------------------------
// Simulation
// ---------------------------------------------------------------------------

// SimulateRace runs one corner-by-corner head-to-head. It is a pure function
// of (track, builds, seed) so the server can store just the seed and replay
// the exact same race for the client, or re-derive it during a dispute.
func SimulateRace(track TrackDef, challenger, opponent Build, seed uint64) RaceResult {
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))

	res := RaceResult{
		TrackID:     track.ID,
		TrackName:   track.Name,
		Seed:        seed,
		ChallRating: math.Round(TrackRating(challenger, track)*10) / 10,
		OppRating:   math.Round(TrackRating(opponent, track)*10) / 10,
		Segments:    make([]SegmentResult, 0, len(track.Segments)),
	}

	// Form drifts across the run, one sequence per racer.
	//
	// A setup's spread is applied here rather than only to per-corner noise,
	// and that is the whole reason it does anything. Corner noise is drawn
	// independently eleven times and averages itself away by the finish; form
	// is correlated across the entire run and does not. If you want a car that
	// is genuinely quick on its day and genuinely wayward on the others, the
	// day is the thing that has to vary.
	//
	// The number of draws is unchanged by the scaling, so replays stay exact.
	n := len(track.Segments)
	challForm := formSequence(rng, n, FormSigma*setupVariance(challenger.Setup).SigmaMult)
	oppForm := formSequence(rng, n, FormSigma*setupVariance(opponent.Setup).SigmaMult)

	var challTotal, oppTotal float64
	lastLeader := 0 // -1 opponent, 0 dead even, 1 challenger

	for i, seg := range track.Segments {
		progress := float64(i) / math.Max(1, float64(n-1))

		// Draw every value unconditionally so the RNG stream stays aligned
		// regardless of branch outcomes — that is what keeps replay exact.
		challNoise := rng.NormFloat64()
		oppNoise := rng.NormFloat64()
		challMisRoll := rng.Float64()
		oppMisRoll := rng.Float64()
		challMisCost := rng.Float64()
		oppMisCost := rng.Float64()

		ct, cMis := segmentTime(challenger, seg, progress, challForm[i], challNoise, challMisRoll, challMisCost)
		ot, oMis := segmentTime(opponent, seg, progress, oppForm[i], oppNoise, oppMisRoll, oppMisCost)

		challTotal += ct
		oppTotal += ot
		gap := oppTotal - challTotal

		leader := 0
		switch {
		case gap > 0.01:
			leader = 1
		case gap < -0.01:
			leader = -1
		}
		if leader != 0 && lastLeader != 0 && leader != lastLeader {
			res.LeadChanges++
		}
		if leader != 0 {
			lastLeader = leader
		}

		res.Segments = append(res.Segments, SegmentResult{
			Index:        i,
			Name:         seg.Name,
			Kind:         seg.Kind,
			ChallTime:    round3(ct),
			OppTime:      round3(ot),
			ChallTotal:   round3(challTotal),
			OppTotal:     round3(oppTotal),
			Gap:          round3(gap),
			ChallMistake: cMis,
			OppMistake:   oMis,
			Note:         segmentNote(seg, cMis, oMis, gap, i, n),
		})
	}

	res.ChallTotal = round3(challTotal)
	res.OppTotal = round3(oppTotal)
	res.ChallWon = challTotal < oppTotal
	res.Margin = round3(math.Abs(oppTotal - challTotal))
	res.Upset = (res.ChallWon && res.ChallRating < res.OppRating) ||
		(!res.ChallWon && res.OppRating < res.ChallRating)
	return res
}

// formSequence builds a per-segment form curve as an AR(1) walk, then rescales
// it so the sum over the run has exactly the spread a single constant draw
// would have had. That separation matters: FormSigma sets who wins how often,
// FormMomentum only sets whether the lead changes hands on the way there.
func formSequence(rng *rand.Rand, n int, sigma float64) []float64 {
	if n <= 0 {
		return nil
	}
	rho := FormMomentum
	out := make([]float64, n)
	out[0] = rng.NormFloat64()
	root := math.Sqrt(1 - rho*rho)
	for i := 1; i < n; i++ {
		out[i] = rho*out[i-1] + root*rng.NormFloat64()
	}

	// Var(sum) of the unit-variance AR(1) series, against n^2 for a constant.
	var v float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			v += math.Pow(rho, math.Abs(float64(i-j)))
		}
	}
	scale := sigma * float64(n) / math.Sqrt(v)
	for i := range out {
		out[i] *= scale
	}
	return out
}

func segmentTime(b Build, seg Segment, progress, form, noise, misRoll, misCost float64) (float64, bool) {
	perf := segmentPerf(b, seg, progress)
	base := seg.BaseSeconds * math.Pow(100/perf, TimeExponent)

	// Below-average consistency widens every distribution; above-average
	// narrows it. Clamped so nobody becomes a metronome or a lottery ticket.
	cons := clamp(ReferenceDriverStat/math.Max(b.Driver.Consistency, 1), 0.55, 1.8)

	// How hard the car is being leaned on. Unlike the performance side of a
	// setup this does not net out — it is the choice between a car that is
	// quick on its day and one that turns up every time.
	sv := setupVariance(b.Setup)

	sigma := (SegmentSigma*(0.5+seg.Risk)*cons + b.Car.TurboLag*TurboLagPenalty) * sv.SigmaMult
	t := base * (1 + form + noise*sigma)
	if t < base*0.55 {
		t = base * 0.55 // nobody gains half a second a corner from luck alone
	}

	// Mistake probability: risk of the corner, sharpened by bravery, blunted
	// by consistency and by knowing where the corner goes.
	brave := clamp(1+(b.Driver.Bravery-ReferenceDriverStat)/ReferenceDriverStat*0.35, 0.6, 1.9)
	smart := clamp(1-(b.Driver.Intelligence-ReferenceDriverStat)/ReferenceDriverStat*0.20, 0.5, 1.6)
	p := MistakeBaseProb * seg.Risk * cons * brave * smart * sv.MistakeMult
	if misRoll < p {
		cost := MistakeCostMin + misCost*(MistakeCostMax-MistakeCostMin)
		return t + seg.BaseSeconds*cost, true
	}
	return t, false
}

func segmentNote(seg Segment, cMis, oMis bool, gap float64, i, n int) string {
	switch {
	case cMis && oMis:
		return "Both cars run wide — contact-close through " + seg.Name + "."
	case cMis:
		return mistakeVerb(seg.Kind) + " — you lose time in " + seg.Name + "."
	case oMis:
		return "Your rival " + mistakeVerbLower(seg.Kind) + " in " + seg.Name + "."
	}
	if i == n-1 {
		if math.Abs(gap) < 0.15 {
			return "Side by side to the line."
		}
		return ""
	}
	if math.Abs(gap) < 0.1 {
		return "Dead even through " + seg.Name + "."
	}
	if math.Abs(gap) > 2.5 {
		if gap > 0 {
			return "You're pulling clear."
		}
		return "The gap ahead is opening up."
	}
	return ""
}

func mistakeVerb(k SegmentKind) string {
	switch k {
	case KindBraking:
		return "Lockup under braking"
	case KindHairpin:
		return "Too much angle on entry"
	case KindSweeper:
		return "A wheel drops off the edge"
	case KindChicane:
		return "You clip the inside kerb"
	case KindStraight:
		return "A missed shift"
	default:
		return "You run out of road"
	}
}

func mistakeVerbLower(k SegmentKind) string {
	switch k {
	case KindBraking:
		return "locks a wheel"
	case KindHairpin:
		return "overshoots the apex"
	case KindSweeper:
		return "drops a wheel wide"
	case KindChicane:
		return "clatters the kerb"
	case KindStraight:
		return "fluffs a shift"
	default:
		return "runs out of road"
	}
}

// PickTrack chooses a track deterministically from a seed.
func PickTrack(seed uint64) TrackDef {
	return Tracks[seed%uint64(len(Tracks))]
}

// NextTrackFor is the road a player is pointed at for their next race.
//
// You are told where you are going before you set the car up, so it has to be
// fixed in advance and it has to stay fixed. Like K's Office stock, it is a
// pure function — of the player and of how many races they have finished —
// rather than a stored column: there is nothing to re-roll by closing the app,
// and it advances only by actually racing.
func NextTrackFor(playerID string, racesCompleted int) TrackDef {
	// FNV-1a over the id, mixed with the race count. Hashing the count in
	// rather than adding it stops the sequence being a predictable rotation
	// through the list.
	var h uint64 = 14695981039346656037
	for i := 0; i < len(playerID); i++ {
		h ^= uint64(playerID[i])
		h *= 1099511628211
	}
	h ^= uint64(racesCompleted) + 0x9E3779B97F4A7C15
	h *= 1099511628211
	h ^= h >> 29
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 32
	return Tracks[h%uint64(len(Tracks))]
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// FormatGap renders a gap the way a timing screen would.
func FormatGap(g float64) string {
	if g >= 0 {
		return fmt.Sprintf("+%.2f", g)
	}
	return fmt.Sprintf("%.2f", g)
}
