package game

import "math"

// ---------------------------------------------------------------------------
// Setup — what the spanner does that the wallet cannot
//
// A setup is not an upgrade. Upgrades add performance; a setup MOVES it, from
// the parts of a road you have decided not to care about onto the parts you
// have. Every axis is centred (see centredEffects) so that its time-weighted
// average across all six tracks is exactly neutral: there is no setting that
// is simply better, and no amount of fiddling raises your overall rating by a
// single point.
//
// That property is what makes the feature interesting rather than a second
// upgrade screen. It also keeps the race calibration honest — the 65% headline
// is measured on neutral setups, and a tuned car has the same average rating
// as a stock one, so the curve cannot drift underneath us.
//
// You can only adjust what you have bought the parts for. A stock car has
// nothing to turn: no coilovers, no adjustable arms, no boost controller. Each
// upgrade tier hands over another spanner, which gives the upgrade ladder a
// second payoff beyond raw numbers.
// ---------------------------------------------------------------------------

// SetupNotch is the range of every axis. Zero is however the car left the
// workshop.
const (
	SetupMin = -2
	SetupMax = 2
)

// Setup is one car's adjustment sheet. Values outside [SetupMin, SetupMax] or
// on an axis the car has no part for are rejected before they ever land here.
type Setup struct {
	Gearing  int `json:"gearing"`
	Boost    int `json:"boost"`
	Pressure int `json:"pressure"`
	Ride     int `json:"ride_height"`
	Camber   int `json:"camber"`
	Damping  int `json:"damping"`
	Ballast  int `json:"ballast"`
}

// SetupAxis describes one adjustment: what unlocks it, what it trades, and how
// to talk about it without printing a spreadsheet at the player.
type SetupAxis struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	// MinTier is the upgrade tier in Category that fits the adjustable part.
	MinTier int `json:"min_tier"`
	// Part names the thing you actually bolted on, so the garage can explain
	// why an axis is locked in the language of the car rather than the schema.
	Part string `json:"part"`
	// Low and High are what the ends of the range mean, in the driver's words.
	Low  string `json:"low"`
	High string `json:"high"`
	Note string `json:"note"`
}

// SetupAxes is the full sheet. Suspension carries three of the seven, because
// suspension is where setup work actually lives.
var SetupAxes = []SetupAxis{
	{
		ID: "gearing", Label: "Final Drive", Category: CatEngine, MinTier: 1,
		Part: "Intake & Exhaust", Low: "Short", High: "Long",
		Note: "Short gears fire you out of slow corners and run out of road on a straight.",
	},
	{
		ID: "boost", Label: "Boost Pressure", Category: CatTurbo, MinTier: 1,
		Part: "Bolt-On Snail", Low: "Low", High: "High",
		Note: "More boost is more everywhere it can reach the ground, and a spikier car everywhere else.",
	},
	{
		ID: "pressure", Label: "Tyre Pressure", Category: CatTires, MinTier: 1,
		Part: "Sport Touring", Low: "Low", High: "High",
		Note: "Low pressure buys grip now and gives it back when the tyres are hot.",
	},
	{
		ID: "ballast", Label: "Weight Balance", Category: CatWeight, MinTier: 1,
		Part: "Stripped Boot", Low: "Rearward", High: "Forward",
		Note: "Where the mass sits decides whether the car stops or drives. Your layout has opinions.",
	},
	{
		ID: "ride_height", Label: "Ride Height", Category: CatSuspension, MinTier: 2,
		Part: "Adjustable Coilovers", Low: "Low", High: "High",
		Note: "Low is quick where the road is smooth and fast, and hopeless where it is neither.",
	},
	{
		ID: "camber", Label: "Camber", Category: CatSuspension, MinTier: 3,
		Part: "Pillow-Ball Arms", Low: "Conservative", High: "Aggressive",
		Note: "Lean the tops of the wheels in for corner grip. The braking zones will notice.",
	},
	{
		ID: "damping", Label: "Damping", Category: CatSuspension, MinTier: 4,
		Part: "2-Way Dampers", Low: "Soft", High: "Stiff",
		Note: "Stiff changes direction the instant you ask. It also finds every bump you hoped to ignore.",
	},
}

// Get reads one axis by ID.
func (s Setup) Get(axis string) (int, bool) {
	switch axis {
	case "gearing":
		return s.Gearing, true
	case "boost":
		return s.Boost, true
	case "pressure":
		return s.Pressure, true
	case "ballast":
		return s.Ballast, true
	case "ride_height":
		return s.Ride, true
	case "camber":
		return s.Camber, true
	case "damping":
		return s.Damping, true
	}
	return 0, false
}

// With returns a copy with one axis changed.
func (s Setup) With(axis string, v int) (Setup, bool) {
	switch axis {
	case "gearing":
		s.Gearing = v
	case "boost":
		s.Boost = v
	case "pressure":
		s.Pressure = v
	case "ballast":
		s.Ballast = v
	case "ride_height":
		s.Ride = v
	case "camber":
		s.Camber = v
	case "damping":
		s.Damping = v
	default:
		return s, false
	}
	return s, true
}

// AxisByID finds an axis definition.
func AxisByID(id string) (SetupAxis, bool) {
	for _, a := range SetupAxes {
		if a.ID == id {
			return a, true
		}
	}
	return SetupAxis{}, false
}

// AxisUnlocked reports whether a car with these tiers has the part fitted.
func AxisUnlocked(a SetupAxis, tiers UpgradeTiers) bool {
	tier, ok := tiers.Get(a.Category)
	return ok && tier >= a.MinTier
}

// LegalSetup strips any axis the car has no part for. It is applied on the way
// in and again on the way out, so selling a car's tier down — or any future
// path that lowers a tier — can never leave a live adjustment behind on a part
// that is no longer fitted.
func LegalSetup(s Setup, tiers UpgradeTiers) Setup {
	out := Setup{}
	for _, a := range SetupAxes {
		if !AxisUnlocked(a, tiers) {
			continue
		}
		v, _ := s.Get(a.ID)
		if v < SetupMin {
			v = SetupMin
		}
		if v > SetupMax {
			v = SetupMax
		}
		out, _ = out.With(a.ID, v)
	}
	return out
}

// ---------------------------------------------------------------------------
// Effects
// ---------------------------------------------------------------------------

// rawEffects is the per-notch performance shift by segment kind, before
// centring. Positive means the high end of the axis helps that kind of road.
//
// These are the design surface: they say what the car is like, not what it is
// worth. What each one is *worth* falls out of the track you are pointed at.
var rawEffects = map[string]map[SegmentKind]float64{
	// Long gears trade corner exit for terminal speed.
	"gearing": {
		KindStraight: +0.030, KindSweeper: +0.014,
		KindHairpin: -0.030, KindTechnical: -0.022, KindChicane: -0.018, KindBraking: -0.004,
	},
	// Boost is power, and power is worth most where the road is open.
	"boost": {
		KindStraight: +0.034, KindSweeper: +0.016, KindBraking: -0.006,
		KindHairpin: -0.014, KindTechnical: -0.018, KindChicane: -0.016,
	},
	// Soft tyres grip and then go off; hard ones roll. Pulled apart across the
	// corner types rather than being uniformly worse than a straight: centring
	// deletes anything that is merely a constant, so an axis earns its keep by
	// disagreeing with itself from one kind of road to the next.
	"pressure": {
		KindStraight: +0.034, KindChicane: -0.008, KindHairpin: -0.012,
		KindTechnical: -0.016, KindBraking: -0.022, KindSweeper: -0.032,
	},
	// Forward mass stops and turns in; rearward mass puts the power down.
	"ballast": {
		KindBraking: +0.030, KindHairpin: +0.020, KindTechnical: +0.010,
		KindStraight: -0.024, KindSweeper: -0.018, KindChicane: -0.002,
	},
	// A low car is a fast car until the road stops being flat.
	"ride_height": {
		KindStraight: +0.018, KindSweeper: +0.026,
		KindHairpin: -0.020, KindTechnical: -0.026, KindChicane: -0.014, KindBraking: -0.006,
	},
	// Camber is corner grip bought with the contact patch under braking — and
	// it is worth most where the load stays on: a long committed sweeper, not
	// a second-gear hairpin you are barely leaning on.
	"camber": {
		KindSweeper: +0.040, KindTechnical: +0.020, KindChicane: +0.014, KindHairpin: +0.006,
		KindStraight: -0.024, KindBraking: -0.034,
	},
	// Stiff dampers change direction; soft ones survive the surface.
	"damping": {
		KindChicane: +0.030, KindTechnical: +0.024, KindHairpin: +0.012,
		KindSweeper: -0.010, KindStraight: -0.008, KindBraking: -0.014,
	},
}

// kindShare is how much of the racing across every track in the game is spent
// on each kind of road, weighted by time — and by how quickly the layout
// covers it.
//
// The drivetrain term matters more than it looks. A rating is a
// performance-weighted average, not a plain one, so centring an axis against
// flat time alone leaves a residue on any car whose performance is not flat
// across corner types. An all-wheel-drive car is quicker on straights and
// slower through hairpins before the setup says anything, and centring that
// ignored the fact handed every AWD driver the same free tenth. Weighting the
// share by drivetrainMod removes it at the source.
var kindShare = computeKindShares()

func computeKindShares() map[Drivetrain]map[SegmentKind]float64 {
	out := map[Drivetrain]map[SegmentKind]float64{}
	for _, dt := range []Drivetrain{FR, FF, MR, AWD} {
		total := 0.0
		share := map[SegmentKind]float64{}
		for _, t := range Tracks {
			for _, seg := range t.Segments {
				w := seg.BaseSeconds * drivetrainMod(seg.Kind, dt)
				share[seg.Kind] += w
				total += w
			}
		}
		if total > 0 {
			for k, v := range share {
				share[k] = v / total
			}
		}
		out[dt] = share
	}
	return out
}

// centredEffects subtracts each axis's own average, so that every axis is a
// pure trade: whatever a notch gives you somewhere, it takes back elsewhere in
// exact proportion to how much of the game's racing happens there.
//
// This is the mechanism behind the no-free-lunch guarantee, and it is
// structural rather than hand-tuned — the numbers above can be edited freely
// without anyone having to re-balance them back to neutral by hand. Weight
// balance has its layout sensitivity folded in before centring rather than
// multiplied on afterwards, so that scaling cannot reintroduce a bias.
var centredEffects = centreAll()

func centreAll() map[Drivetrain]map[string]map[SegmentKind]float64 {
	out := map[Drivetrain]map[string]map[SegmentKind]float64{}
	for dt, share := range kindShare {
		perAxis := make(map[string]map[SegmentKind]float64, len(rawEffects))
		for axis, eff := range rawEffects {
			scale := 1.0
			if axis == "ballast" {
				scale = ballastLayout(dt)
			}
			mean := 0.0
			for kind, sh := range share {
				mean += eff[kind] * scale * sh
			}
			centred := make(map[SegmentKind]float64, len(eff))
			for kind := range share {
				centred[kind] = eff[kind]*scale - mean
			}
			perAxis[axis] = centred
		}
		out[dt] = perAxis
	}
	return out
}

// effectsFor is the centred table for a layout, falling back to rear-drive for
// anything unrecognised rather than silently zeroing every adjustment.
func effectsFor(dt Drivetrain) map[string]map[SegmentKind]float64 {
	if e, ok := centredEffects[dt]; ok {
		return e
	}
	return centredEffects[FR]
}

// setupMod is the performance multiplier a setup applies to one kind of road.
//
// Layout matters for weight balance and only for weight balance: shifting mass
// forward in a car that is already nose-heavy is not the same move as doing it
// in a mid-engined one. It is the one place where the right answer depends on
// what you are driving rather than where.
// setupModRaw is the shape of the trade before it is made honest for a
// particular car. See SetupNorm for why that second step exists.
func setupModRaw(s Setup, kind SegmentKind, dt Drivetrain) float64 {
	if s == (Setup{}) {
		return 1 // the overwhelmingly common case, and exactly neutral
	}
	e := effectsFor(dt)
	mod := 1.0
	mod += float64(s.Gearing) * e["gearing"][kind]
	mod += float64(s.Boost) * e["boost"][kind]
	mod += float64(s.Pressure) * e["pressure"][kind]
	mod += float64(s.Ride) * e["ride_height"][kind]
	mod += float64(s.Camber) * e["camber"][kind]
	mod += float64(s.Damping) * e["damping"][kind]
	mod += float64(s.Ballast) * e["ballast"][kind]

	// A setup can shade a corner, never transform it.
	return clamp(mod, 0.86, 1.14)
}

// setupNormFor is what keeps "a setup cannot make you faster" true for every
// car rather than only for an average one.
//
// Centring the effects makes the modifier average to one across the game's
// roads. A rating is not that average though — it is weighted by how well the
// car goes on each kind of road, and a fully built car is nothing like flat.
// Pour 120hp into an engine and straights start carrying more of your rating
// than they used to, so a sheet wound towards straights quietly gained about
// 1.7% overall on a maxed car: a free lunch, and worse, a single obvious
// answer for every player who found it.
//
// Dividing by the build's own performance-weighted average of the modifier
// removes it exactly, whatever the car. It costs one pass over the six tracks
// per build, which happens once per race rather than once per corner.
func setupNormFor(car CarStats, driver DriverStats, s Setup) float64 {
	if s == (Setup{}) {
		return 1
	}
	bare := Build{Car: car, Driver: driver} // SetupNorm unused: no setup to scale
	var num, den float64
	for _, t := range Tracks {
		n := len(t.Segments)
		for i, seg := range t.Segments {
			progress := float64(i) / math.Max(1, float64(n-1))
			p := segmentPerf(bare, seg, progress) * seg.BaseSeconds
			num += p * setupModRaw(s, seg.Kind, car.Drivetrain)
			den += p
		}
	}
	if den == 0 || num == 0 {
		return 1
	}
	return num / den
}

// WithSetup attaches a sheet to a build and works out the constant that keeps
// it honest. Every build that races or gets rated goes through here — a build
// assembled without it would be scored against an uncentred setup.
func WithSetup(b Build, s Setup) Build {
	b.Setup = s
	b.SetupNorm = setupNormFor(b.Car, b.Driver, s)
	return b
}

// setupMod is the honest modifier: the trade, divided by whatever that trade
// was worth on average to this particular car.
func setupMod(b Build, kind SegmentKind) float64 {
	raw := setupModRaw(b.Setup, kind, b.Car.Drivetrain)
	if b.SetupNorm <= 0 {
		// A build assembled by hand rather than through WithSetup. Falling
		// back to the uncentred value is wrong but bounded; returning zero
		// would delete the car.
		return raw
	}
	return raw / b.SetupNorm
}

// ballastLayout scales weight transfer by where the engine already is.
func ballastLayout(dt Drivetrain) float64 {
	switch dt {
	case FF:
		// Already nose-heavy: adding more forward bias does little good and
		// taking it out helps more than it would elsewhere.
		return 0.7
	case MR:
		// Mid-engined cars are the most sensitive thing here to move mass in.
		return 1.35
	case AWD:
		return 0.9
	}
	return 1.0 // FR
}

// SetupVariance is what a setup does to how repeatable the car is. Unlike the
// performance shift this is deliberately NOT centred: leaning on the car makes
// it faster on its day and messier on the others.
//
// It is the part of the sheet that depends on who you are racing rather than
// where. Behind on the ladder, spikiness is the only thing that beats a better
// car; ahead of the field, it is how you lose to one.
type SetupVariance struct {
	// SigmaMult scales per-corner noise.
	SigmaMult float64
	// MistakeMult scales the chance of getting one wrong.
	MistakeMult float64
}

// The two knobs here are not the same kind of thing, and the difference is
// what makes risk a strategy rather than a tax.
//
// Spread is symmetric: a wider distribution is worth having when you are the
// slower car, because the only way past a better build is a run that goes
// unusually well, and it costs the faster car for exactly the same reason.
// Mistakes are not symmetric — a lockup is time lost and never time found — so
// they are pure cost to whoever makes them.
//
// A setup that raised both equally would just be slower, and nobody would ever
// choose it. Aggression therefore moves spread hard and mistakes gently: it
// genuinely buys upsets, and it genuinely throws away races you were winning.
func setupVariance(s Setup) SetupVariance {
	v := SetupVariance{SigmaMult: 1, MistakeMult: 1}

	// Boost: everything arrives at once, or not yet.
	v.SigmaMult += float64(s.Boost) * 0.100
	v.MistakeMult += float64(s.Boost) * 0.015

	// Camber: quick, until there is nothing left standing on the tyre.
	v.SigmaMult += float64(s.Camber) * 0.030
	v.MistakeMult += float64(s.Camber) * 0.030

	// Stiff dampers give up compliance; soft ones absorb a bad line.
	v.SigmaMult += float64(s.Damping) * 0.060
	v.MistakeMult += float64(s.Damping) * 0.010

	// Low pressures are grippier and less predictable as they heat.
	v.SigmaMult -= float64(s.Pressure) * 0.025

	v.SigmaMult = clamp(v.SigmaMult, 0.75, 1.45)
	v.MistakeMult = clamp(v.MistakeMult, 0.80, 1.35)
	return v
}

// ---------------------------------------------------------------------------
// Describing a setup without printing the model at the player
// ---------------------------------------------------------------------------

// SetupHint is a qualitative read on what an axis is doing for a given track —
// enough to reason with, not enough to solve. The player is told the shape of
// the trade, never the coefficient.
type SetupHint struct {
	AxisID string `json:"axis_id"`
	Value  int    `json:"value"`
	// Effect is one of "strong", "slight", "neutral", "poor", "wrong".
	Effect string `json:"effect"`
}

// HintFor reads how an axis's current setting suits a track, by asking the
// model the same question the race will ask it.
func HintFor(axis string, value int, t TrackDef, dt Drivetrain) SetupHint {
	hint := SetupHint{AxisID: axis, Value: value, Effect: "neutral"}
	if value == 0 {
		return hint
	}
	eff, ok := effectsFor(dt)[axis]
	if !ok {
		return hint
	}

	// Time-weighted shift this axis alone produces on this track.
	var num, den float64
	for _, seg := range t.Segments {
		num += eff[seg.Kind] * float64(value) * seg.BaseSeconds
		den += seg.BaseSeconds
	}
	if den == 0 {
		return hint
	}
	switch shift := num / den; {
	case shift > 0.012:
		hint.Effect = "strong"
	case shift > 0.003:
		hint.Effect = "slight"
	case shift < -0.012:
		hint.Effect = "wrong"
	case shift < -0.003:
		hint.Effect = "poor"
	}
	return hint
}

// SetupSpread is how far apart the best and worst settings of an axis are on a
// given track. Near zero means this track does not care what you do here —
// which is worth knowing, and is not the same as the axis being useless.
func SetupSpread(axis string, t TrackDef, dt Drivetrain) float64 {
	eff := effectsFor(dt)[axis]
	best := math.Inf(-1)
	worst := math.Inf(1)
	for v := SetupMin; v <= SetupMax; v++ {
		h := 0.0
		var den float64
		for _, seg := range t.Segments {
			h += eff[seg.Kind] * float64(v) * seg.BaseSeconds
			den += seg.BaseSeconds
		}
		if den > 0 {
			h /= den
		}
		best = math.Max(best, h)
		worst = math.Min(worst, h)
	}
	return best - worst
}
