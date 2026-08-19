package game

import (
	"math"
	"testing"
)

// The whole feature rests on one claim: a setup moves performance around, it
// never creates any. If that ever stops being true, tuning becomes a second
// upgrade screen, the rating stops meaning what the garage says it means, and
// the 65% calibration drifts out from under the race model.

func allTiers(n int) UpgradeTiers {
	return UpgradeTiers{Engine: n, Turbo: n, Tires: n, Suspension: n, Weight: n}
}

// everySetup walks a representative sweep of the adjustment space. The full
// cross product is 5^7; this hits every axis at every notch plus the corners.
func everySetup() []Setup {
	out := []Setup{{}}
	for _, a := range SetupAxes {
		for v := SetupMin; v <= SetupMax; v++ {
			if v == 0 {
				continue
			}
			s, _ := Setup{}.With(a.ID, v)
			out = append(out, s)
		}
	}
	// Everything wound one way, then the other, then a mixed sheet.
	out = append(out,
		Setup{Gearing: 2, Boost: 2, Pressure: 2, Ride: 2, Camber: 2, Damping: 2, Ballast: 2},
		Setup{Gearing: -2, Boost: -2, Pressure: -2, Ride: -2, Camber: -2, Damping: -2, Ballast: -2},
		Setup{Gearing: 2, Boost: -2, Pressure: 1, Ride: -1, Camber: 2, Damping: -2, Ballast: 1},
	)
	return out
}

func TestSetupCannotRaiseOverallRating(t *testing.T) {

	for _, dt := range []Drivetrain{FR, FF, MR, AWD} {
		// Each layout is measured against its own factory rating: a drivetrain
		// changes what a car is worth before the setup says anything, and that
		// is not what this test is about.
		neutral := referenceBuild()
		neutral.Car.Drivetrain = dt
		base := OverallRating(neutral)

		for _, s := range everySetup() {
			b := referenceBuild()
			b.Car.Drivetrain = dt
			b = WithSetup(b, s)
			got := OverallRating(b)
			// Centring makes this exact in the model; the tolerance is for the
			// clamp in setupMod and for floating point, not for slack.
			if math.Abs(got-base) > 0.05 {
				t.Errorf("setup %+v on %s rates %.4f, want %.4f — a setup must not add performance",
					s, dt, got, base)
			}
		}
	}
}

// The flip side: if no setup helps anywhere, the feature does nothing. A
// setup has to matter a lot on the right road.
func TestSetupMattersOnASpecificTrack(t *testing.T) {
	akina, ok := TrackByID("akina_down") // five consecutive hairpins
	if !ok {
		t.Fatal("akina_down missing")
	}
	happo, ok := TrackByID("happogahara") // high-speed sweepers
	if !ok {
		t.Fatal("happogahara missing")
	}

	// +2 is the short end of the final drive (4.50), -2 the long one (3.70).
	short := referenceBuild()
	short = WithSetup(short, Setup{Gearing: 2})
	long := referenceBuild()
	long = WithSetup(long, Setup{Gearing: -2})

	// Short gears on the hairpins, long gears on the fast one.
	if a, b := TrackRating(short, akina), TrackRating(long, akina); a <= b {
		t.Errorf("short gearing rates %.3f on Akina vs long %.3f — short should win here", a, b)
	}
	if a, b := TrackRating(short, happo), TrackRating(long, happo); a >= b {
		t.Errorf("short gearing rates %.3f on Happogahara vs long %.3f — long should win here", a, b)
	}

	// And the swing has to be big enough to be worth thinking about.
	swing := TrackRating(short, akina) - TrackRating(long, akina)
	if swing < 0.5 {
		t.Errorf("gearing swings only %.3f rating on Akina — too small to be a decision", swing)
	}
}

// No setting may be right everywhere: for every axis, the setting that wins on
// one track has to lose on another. This is the "no meta" property stated as a
// test rather than as an intention.
func TestNoAxisHasAUniversallyCorrectSetting(t *testing.T) {
	for _, a := range SetupAxes {
		best := map[float64]int{}
		for _, track := range Tracks {
			bestV, bestScore := 0.0, math.Inf(-1)
			for v := SetupMin; v <= SetupMax; v++ {
				b := referenceBuild()
				sv, _ := Setup{}.With(a.ID, v)
				b = WithSetup(b, sv)
				if s := TrackRating(b, track); s > bestScore {
					bestV, bestScore = v, s
				}
			}
			best[bestV]++
		}
		if len(best) < 2 {
			t.Errorf("axis %q has the same best setting on every track (%v) — that is a meta, not a choice",
				a.ID, best)
		}
	}
}

// Every axis must actually be reachable, and locked until its part is fitted.
func TestSetupAxesAreGatedByParts(t *testing.T) {
	stock := UpgradeTiers{}
	for _, a := range SetupAxes {
		if AxisUnlocked(a, stock) {
			t.Errorf("axis %q is adjustable on a completely stock car", a.ID)
		}
		if _, ok := UpgradeCost(a.Category, a.MinTier); !ok {
			t.Errorf("axis %q is gated on %s tier %d, which is not a real upgrade",
				a.ID, a.Category, a.MinTier)
		}
	}
	if !AxisUnlocked(SetupAxes[0], allTiers(MaxTier)) {
		t.Error("a fully built car cannot adjust its first axis")
	}

	// Suspension carries the most adjustment, as the design intends.
	byCat := map[string]int{}
	for _, a := range SetupAxes {
		byCat[a.Category]++
	}
	for cat, n := range byCat {
		if cat != CatSuspension && n >= byCat[CatSuspension] {
			t.Errorf("%s has %d axes and suspension has %d — suspension should be the deepest",
				cat, n, byCat[CatSuspension])
		}
	}
}

func TestLegalSetupStripsUnfittedParts(t *testing.T) {
	wound := Setup{Gearing: 2, Boost: 2, Pressure: 2, Ride: 2, Camber: 2, Damping: 2, Ballast: 2}

	if got := LegalSetup(wound, UpgradeTiers{}); !got.IsFactory() {
		t.Errorf("stock car kept adjustments %+v", got)
	}

	// Coilovers but no arms and no dampers: ride height only.
	partial := LegalSetup(wound, UpgradeTiers{Suspension: 2})
	if partial.Ride != 2 {
		t.Errorf("ride height should be adjustable on coilovers, got %v", partial.Ride)
	}
	if partial.Camber != 0 || partial.Damping != 0 {
		t.Errorf("camber/damping adjustable without the parts: %+v", partial)
	}

	// Out-of-range values are pulled back rather than trusted.
	if got := LegalSetup(Setup{Gearing: 99}, allTiers(MaxTier)); got.Gearing != SetupMax {
		t.Errorf("gearing 99 survived as %v, want clamped to %v", got.Gearing, SetupMax)
	}
	if got := LegalSetup(Setup{Gearing: -99}, allTiers(MaxTier)); got.Gearing != SetupMin {
		t.Errorf("gearing -99 survived as %v, want clamped to %v", got.Gearing, SetupMin)
	}
	// Nothing finer than the workshop's own precision survives either: an axis
	// is continuous, not infinitely divisible.
	fine := LegalSetup(Setup{Ride: 0.5137}, allTiers(MaxTier))
	ride, _ := AxisByID("ride_height")
	if got := ride.Setting(fine.Ride); got != math.Round(got) {
		t.Errorf("a ride height of %vmm survived; the collar moves in millimetres", got)
	}
	// And a sheet nobody has touched stays untouched through the round trip.
	if !LegalSetup(Setup{}, allTiers(MaxTier)).IsFactory() {
		t.Error("a factory sheet came back tuned")
	}
}

// The axes are physical: a position converts to a real number and back without
// drifting, which is what lets a player type 87 and get 87.
func TestAxisConvertsBothWays(t *testing.T) {
	for _, a := range SetupAxes {
		if a.Step <= 0 {
			t.Errorf("axis %q has no step — it cannot say how finely it adjusts", a.ID)
			continue
		}
		if a.Setting(0) != a.Factory() {
			t.Errorf("axis %q reads %v at the factory position, want %v", a.ID, a.Setting(0), a.Factory())
		}
		// Walk the whole range in single steps.
		lo, hi := math.Min(a.SettingLow(), a.SettingHigh()), math.Max(a.SettingLow(), a.SettingHigh())
		for v := lo; v <= hi+1e-9; v += a.Step {
			pos := a.PositionFor(v)
			if pos < SetupMin || pos > SetupMax {
				t.Fatalf("axis %q put %v at position %v", a.ID, v, pos)
			}
			if got := a.Setting(pos); math.Abs(got-v) > a.Step/100 {
				t.Errorf("axis %q: %v became %v on the way round", a.ID, v, got)
			}
			if got := a.Setting(a.Quantize(pos)); math.Abs(got-v) > a.Step/2 {
				t.Errorf("axis %q: quantising %v moved it to %v", a.ID, v, got)
			}
		}
		// Off both ends, clamped rather than extrapolated.
		if a.Setting(a.PositionFor(hi+1000)) != a.SettingHigh() && a.Setting(a.PositionFor(hi+1000)) != a.SettingLow() {
			t.Errorf("axis %q extrapolated past the end of its range", a.ID)
		}
		if a.InRange(hi + 10*a.Step) {
			t.Errorf("axis %q accepts %v, which is off the sheet", a.ID, hi+10*a.Step)
		}
	}
}

// Leaning on the car has to actually make it spikier, or the risk axis of the
// design does not exist.
func TestAggressiveSetupsAreLessRepeatable(t *testing.T) {
	calm := setupVariance(Setup{Boost: -2, Camber: -2, Damping: -2})
	wild := setupVariance(Setup{Boost: 2, Camber: 2, Damping: 2})

	if wild.SigmaMult <= calm.SigmaMult {
		t.Errorf("aggressive sigma %.3f is not above calm %.3f", wild.SigmaMult, calm.SigmaMult)
	}
	if wild.MistakeMult <= calm.MistakeMult {
		t.Errorf("aggressive mistake rate %.3f is not above calm %.3f", wild.MistakeMult, calm.MistakeMult)
	}
	if neutral := setupVariance(Setup{}); neutral.SigmaMult != 1 || neutral.MistakeMult != 1 {
		t.Errorf("a factory setup should be exactly neutral, got %+v", neutral)
	}
}

// A spiky setup is how an underdog steals one. Same rating gap, more variance,
// more upsets — which is the strategic point of the risk axes.
func TestVarianceHelpsTheUnderdog(t *testing.T) {
	fav := edgeBuild(t, 0.05)
	calm := referenceBuild()
	wild := referenceBuild()
	wild = WithSetup(wild, Setup{Boost: 2, Camber: 2, Damping: 2})

	// The favourite is on a factory sheet in both runs.
	calmRate := 1 - winRate(t, fav, calm, 60000)
	wildRate := 1 - winRate(t, fav, wild, 60000)

	if wildRate <= calmRate {
		t.Errorf("underdog wins %.2f%% on a wild setup vs %.2f%% calm — spikiness should buy upsets",
			wildRate*100, calmRate*100)
	}
	t.Logf("underdog win rate: calm %.2f%%, wild %.2f%%", calmRate*100, wildRate*100)
}

// The headline number has to survive both cars being tuned.
func TestCalibrationHoldsWithTunedCars(t *testing.T) {
	fav := edgeBuild(t, 0.05)
	dog := referenceBuild()
	fav = WithSetup(fav, Setup{Gearing: 1, Pressure: -1, Ride: -1})
	dog = WithSetup(dog, Setup{Gearing: -1, Camber: 1, Ballast: 1})

	rate := winRate(t, fav, dog, 120000)
	if rate < 0.62 || rate > 0.68 {
		t.Errorf("a +5%% favourite wins %.2f%% with both cars tuned, want ~65%%", rate*100)
	}
	t.Logf("+5%% favourite with both cars tuned: %.2f%%", rate*100)
}

// The next track has to be knowable in advance and stable, or there is nothing
// to tune for — and it must not be re-rollable by asking again.
func TestNextTrackIsStableUntilYouRace(t *testing.T) {
	first := NextTrackFor("player-one", 7)
	for i := 0; i < 50; i++ {
		if got := NextTrackFor("player-one", 7); got.ID != first.ID {
			t.Fatalf("next track changed from %s to %s without a race", first.ID, got.ID)
		}
	}
	// Racing moves you on.
	changed := false
	for n := 8; n < 8+len(Tracks)*3; n++ {
		if NextTrackFor("player-one", n).ID != first.ID {
			changed = true
			break
		}
	}
	if !changed {
		t.Error("the same track comes up forever")
	}
	// Two players are not marched through the same order.
	same := 0
	for n := 0; n < 40; n++ {
		if NextTrackFor("player-one", n).ID == NextTrackFor("player-two", n).ID {
			same++
		}
	}
	if same == 40 {
		t.Error("every player sees the same track sequence")
	}
}

// Over a long run every track has to come up, or some of the game is unseen.
func TestNextTrackReachesEveryTrack(t *testing.T) {
	seen := map[string]bool{}
	for n := 0; n < 4000; n++ {
		seen[NextTrackFor("some-player-id", n).ID] = true
	}
	if len(seen) != len(Tracks) {
		t.Errorf("saw %d of %d tracks over 4000 races", len(seen), len(Tracks))
	}
}

// ---------------------------------------------------------------------------
// What a setup is worth
// ---------------------------------------------------------------------------

// bestSetupFor hill-climbs the sheet for a track, one axis at a time, which is
// what a determined player does by hand. Coordinate ascent rather than the full
// cross product: it lands on the same answer for a model this smooth and is
// fast enough to run in a test.
//
// It sweeps each axis coarsely and then refines, because the axes are
// continuous now — searching the five old notches would flatter the model by
// measuring only the settings it used to allow, and the player with a text box
// is not so restricted.
func bestSetupFor(b Build, t TrackDef) Setup {
	best := Setup{}
	bestScore := TrackRating(WithSetup(b, best), t)
	for _, step := range []float64{0.25, 0.05} {
		for pass := 0; pass < 4; pass++ {
			improved := false
			for _, a := range SetupAxes {
				at, _ := best.Get(a.ID)
				lo, hi := SetupMin, SetupMax
				if step < 0.25 {
					lo, hi = math.Max(SetupMin, at-0.25), math.Min(SetupMax, at+0.25)
				}
				for v := lo; v <= hi+1e-9; v += step {
					cand, _ := best.With(a.ID, a.Quantize(v))
					if s := TrackRating(WithSetup(b, cand), t); s > bestScore+1e-9 {
						best, bestScore = cand, s
						improved = true
					}
				}
			}
			if !improved {
				break
			}
		}
	}
	return best
}

// The headline number for the whole feature: getting the car right for the
// road should be worth about a ladder rung, and no more. Too little and the
// spanner is decoration; too much and the garage stops mattering.
//
// And it has to be worth about the same everywhere. A pass that pays twice
// what the others pay is the pass everyone tunes for and nobody tunes on the
// rest — Happogahara used to pay 7.8% against a 3.5% floor, which is two
// different games depending on which road came up.
func TestPerfectSetupIsWorthAboutFivePercent(t *testing.T) {
	base := referenceBuild()
	var sum float64
	low, high := math.Inf(1), math.Inf(-1)
	lowName, highName := "", ""
	for _, track := range Tracks {
		factory := TrackRating(WithSetup(base, Setup{}), track)
		tuned := TrackRating(WithSetup(base, bestSetupFor(base, track)), track)
		gain := tuned/factory - 1
		sum += gain
		t.Logf("%-28s perfect setup worth %+.2f%%", track.Name, gain*100)
		if gain < 0.03 {
			t.Errorf("%s: a perfect setup is worth only %+.2f%% — not worth the screen", track.Name, gain*100)
		}
		if gain > 0.07 {
			t.Errorf("%s: a perfect setup is worth %+.2f%% — the sheet outweighs the build", track.Name, gain*100)
		}
		if gain < low {
			low, lowName = gain, track.Name
		}
		if gain > high {
			high, highName = gain, track.Name
		}
	}
	if avg := sum / float64(len(Tracks)); avg < 0.04 || avg > 0.06 {
		t.Errorf("a perfect setup averages %+.2f%% across the game, want about 5%%", avg*100)
	} else {
		t.Logf("average across the game: %+.2f%%", avg*100)
	}
	if spread := high / low; spread > 1.6 {
		t.Errorf("%s pays %.2f%% and %s pays %.2f%% — %.2fx, so only one of them is really a tuning screen",
			highName, high*100, lowName, low*100, spread)
	} else {
		t.Logf("most setup-sensitive pass pays %.2fx the least (%s vs %s)", spread, highName, lowName)
	}
}

// A sheet built for one pass has to be actively wrong on another, or players
// would set the car up once and never touch it again.
func TestASetupForOnePassIsWrongOnAnother(t *testing.T) {
	base := referenceBuild()
	worseSomewhere := 0
	for _, home := range Tracks {
		sheet := bestSetupFor(base, home)
		var carriedMean float64
		n := 0
		for _, away := range Tracks {
			if away.ID == home.ID {
				continue
			}
			factory := TrackRating(WithSetup(base, Setup{}), away)
			carried := TrackRating(WithSetup(base, sheet), away)
			carriedMean += carried/factory - 1
			n++
			if carried < factory {
				worseSomewhere++
			}
		}
		// Carrying a sheet to the wrong pass has to cost on average. This is
		// the property that stops a player setting the car up once in their
		// career and never opening the screen again.
		if carriedMean/float64(n) >= 0 {
			t.Errorf("the %s sheet averages %+.2f%% on foreign passes — it travels for free",
				home.Name, carriedMean/float64(n)*100)
		}
	}
	total := len(Tracks) * (len(Tracks) - 1)
	if worseSomewhere < total/2 {
		t.Errorf("a track-specific sheet is worse than factory on only %d of %d other passes",
			worseSomewhere, total)
	}
	t.Logf("track-specific sheets are worse than factory on %d of %d foreign passes", worseSomewhere, total)
}

// And the optimum has to depend on the car, not just the road — otherwise one
// published sheet per pass serves the entire playerbase.
func TestTheBestSetupDependsOnTheCar(t *testing.T) {
	layouts := []Drivetrain{FR, FF, MR, AWD}

	// Every road has to care which end drives, and weight balance is the axis
	// that carries it: mass over the driven wheels is traction, so on any
	// given pass at least one layout has to want its mass forward and at least
	// one has to want it back.
	//
	// Three of the six used to fail this. A descent was modelled as a braking
	// problem and nothing else, so every layout wanted its mass forward for
	// the same reason and half the game had one right answer. What was missing
	// is that gravity unloads the rear of a car going down and the front of
	// one going up, which is precisely when the driven axle should have the
	// loudest opinion — see drivenAxleGrip.
	blind := []string{}
	for _, track := range Tracks {
		forward, back := []Drivetrain{}, []Drivetrain{}
		for _, dt := range layouts {
			b := referenceBuild()
			b.Car.Drivetrain = dt
			switch bal := bestSetupFor(b, track).Ballast; {
			case bal > 0:
				forward = append(forward, dt)
			case bal < 0:
				back = append(back, dt)
			}
		}
		if len(forward) == 0 || len(back) == 0 {
			blind = append(blind, track.Name)
			continue
		}
		t.Logf("%-28s forward %v   back %v", track.Name, forward, back)
	}
	if len(blind) > 0 {
		t.Errorf("every layout wants the mass in the same place on %v — those roads do not care what you drive", blind)
	}

	// On Akina in particular, the five hairpin exits are traction, so the
	// front-driven car is the one that wants the weight over its driven axle.
	akina, _ := TrackByID("akina_down")
	ballast := map[Drivetrain]float64{}
	for _, dt := range layouts {
		b := referenceBuild()
		b.Car.Drivetrain = dt
		ballast[dt] = bestSetupFor(b, akina).Ballast
	}
	if ballast[FF] <= 0 || ballast[FR] >= 0 {
		t.Errorf("on Akina front-drive wants %+.2f and rear-drive %+.2f; the driven axle should split them",
			ballast[FF], ballast[FR])
	}

	// A powerful car and a nimble one should also disagree.
	powerful := referenceBuild()
	powerful.Car.PowerToWt *= 1.6
	nimble := referenceBuild()
	nimble.Car.Grip *= 1.4
	nimble.Car.Handling *= 1.3
	if bestSetupFor(powerful, akina) == bestSetupFor(nimble, akina) {
		t.Error("a power car and a grip car want the identical sheet")
	}
}

// ---------------------------------------------------------------------------
// Crashing
// ---------------------------------------------------------------------------

// crashRate runs a build down a pass many times and reports how often it ends
// against the barrier.
func crashRate(b, rival Build, t TrackDef, c Conditions, runs int) float64 {
	road := ApplyConditions(t, c)
	crashes := 0
	for i := 0; i < runs; i++ {
		r := SimulateRace(road, b, rival, uint64(i)*6364136223846793005+1)
		if r.ChallCrashed {
			crashes++
		}
	}
	return float64(crashes) / float64(runs)
}

// A car on the settings it left the workshop with should get down the mountain.
// A sheet with nothing left in reserve, on a wet night, on the nastiest pass in
// the game, should be a genuine gamble — and the difference between those two
// is what makes an aggressive setup a decision.
func TestCrashesArePricedForRisk(t *testing.T) {
	usui, _ := TrackByID("usui")
	happo, _ := TrackByID("happogahara")
	rival := referenceBuild()

	factory := referenceBuild()
	wild := WithSetup(referenceBuild(),
		Setup{Ride: 2, Damping: 2, Camber: 2, Boost: 2, Pressure: -2})

	dry := Dry()
	wet := Conditions{Damp: 0.8, TempC: 6}

	factoryDry := crashRate(factory, rival, usui, dry, 4000)
	factoryWet := crashRate(factory, rival, usui, wet, 4000)
	wildWet := crashRate(wild, rival, usui, wet, 4000)
	wildWide := crashRate(wild, rival, happo, wet, 4000)

	t.Logf("Usui  factory dry %.2f%%   factory wet %.2f%%   wild wet %.2f%%", factoryDry*100, factoryWet*100, wildWet*100)
	t.Logf("Happo wild wet %.2f%%", wildWide*100)

	if factoryDry > 0.02 {
		t.Errorf("a factory car crashes %.2f%% of the time on a dry pass — far too often", factoryDry*100)
	}
	if wildWet <= factoryWet {
		t.Errorf("a wild sheet crashes %.2f%% against a factory %.2f%% — aggression costs nothing",
			wildWet*100, factoryWet*100)
	}
	if wildWet > 0.35 {
		t.Errorf("a wild sheet crashes %.2f%% of the time — that is not a gamble, it is a wall", wildWet*100)
	}
	// A wide fast pass has to be more forgiving of the same sheet than a
	// narrow broken one, or geometry is not reaching the crash model.
	if wildWide >= wildWet {
		t.Errorf("the same sheet crashes %.2f%% on wide Happogahara vs %.2f%% on narrow Usui",
			wildWide*100, wildWet*100)
	}
}

// Consistency is the stat that keeps a car on the road, so it has to pay here.
func TestAConsistentDriverCrashesLess(t *testing.T) {
	usui, _ := TrackByID("usui")
	wet := Conditions{Damp: 0.8, TempC: 6}
	sheet := Setup{Ride: 2, Damping: 2, Camber: 2}

	sloppy := referenceBuild()
	sloppy.Driver.Consistency = 6
	sloppy = WithSetup(sloppy, sheet)

	steady := referenceBuild()
	steady.Driver.Consistency = 24
	steady = WithSetup(steady, sheet)

	a := crashRate(sloppy, referenceBuild(), usui, wet, 4000)
	b := crashRate(steady, referenceBuild(), usui, wet, 4000)
	t.Logf("same sheet, sloppy driver %.2f%% vs consistent %.2f%%", a*100, b*100)
	if b >= a {
		t.Errorf("consistency bought nothing: %.2f%% vs %.2f%%", b*100, a*100)
	}
}

// The weather has to change what the car wants, or conditions are decoration.
func TestTheWetWantsADifferentSheet(t *testing.T) {
	akagi, _ := TrackByID("akagi_down")
	base := referenceBuild()

	dry := bestSetupFor(base, ApplyConditions(akagi, Dry()))
	wet := bestSetupFor(base, ApplyConditions(akagi, Conditions{Damp: 0.8, TempC: 6}))

	if dry == wet {
		t.Errorf("the same sheet is optimal wet and dry: %+v", dry)
	}
	t.Logf("dry %+v", dry)
	t.Logf("wet %+v", wet)
}

// ---------------------------------------------------------------------------
// Provenance
// ---------------------------------------------------------------------------

// Every pass has to say where its geometry came from. "Authored" and
// "surveyed" are different claims and the difference should be legible from
// the data rather than from a comment somebody might not read.
func TestEveryTrackDeclaresItsSource(t *testing.T) {
	for _, tr := range Tracks {
		switch tr.Source {
		case SourceAuthored, SourceSurveyed:
		default:
			t.Errorf("%s has source %q, want authored or surveyed", tr.ID, tr.Source)
		}
		if tr.RealWorld == "" {
			t.Errorf("%s does not name the road it stands for, so nobody can go and record it", tr.ID)
		}
	}
}

// The switchover has to work before there is anything to switch over to,
// because by the time there is, nobody will be testing the mechanism.
func TestSurveyedGeometryTakesOver(t *testing.T) {
	authored := []TrackDef{
		{ID: "akina_down", Name: "Stand-in", Flavour: "authored flavour",
			Source: SourceAuthored, RealWorld: "Mt. Haruna", Downhill: true,
			Segments: []Segment{{Name: "Guess", Radius: 16, Length: 60, Width: 6, Surface: SurfaceCoarse}}},
		{ID: "usui", Name: "Untouched", Flavour: "still authored",
			Source: SourceAuthored, RealWorld: "Usui Pass",
			Segments: []Segment{{Name: "Guess", Radius: 20, Length: 60, Width: 5, Surface: SurfaceBroken}}},
	}
	surveyed := []TrackDef{
		{ID: "akina_down", Name: "Mt. Haruna — Downhill",
			Segments: []Segment{{Name: "Measured", Radius: 18.4, Length: 63, Width: 6.2, Surface: SurfaceSmooth}}},
	}

	got := mergeSurveyed(authored, surveyed)
	if len(got) != 2 {
		t.Fatalf("merge produced %d tracks, want 2", len(got))
	}

	byID := map[string]TrackDef{}
	for _, tr := range got {
		byID[tr.ID] = tr
	}

	measured := byID["akina_down"]
	if measured.Source != SourceSurveyed {
		t.Errorf("a measured track still reports %q", measured.Source)
	}
	if measured.Segments[0].Radius != 18.4 {
		t.Errorf("the authored radius survived: %.1f", measured.Segments[0].Radius)
	}
	// The things a GPS cannot know are kept from the authored version.
	if measured.Flavour != "authored flavour" {
		t.Errorf("flavour lost on merge: %q", measured.Flavour)
	}
	if measured.RealWorld == "" {
		t.Error("real-world reference lost on merge")
	}
	if !measured.Downhill {
		t.Error("downhill lost on merge — a GPS trace does not know which way the game runs it")
	}

	// A road nobody recorded is left exactly as it was.
	if byID["usui"].Source != SourceAuthored || byID["usui"].Segments[0].Radius != 20 {
		t.Error("an unrecorded road was disturbed by the merge")
	}

	// And with nothing surveyed at all, nothing changes.
	if same := mergeSurveyed(authored, nil); len(same) != 2 || same[0].Source != SourceAuthored {
		t.Error("merging no surveys changed the authored tracks")
	}
}

// An OSM way has coordinates and no elevation, so a road imported from
// Overpass alone is perfectly flat — and a flat Irohazaka is not a small
// error, because gradient drives braking distance, the crash model and which
// setup wins.
func TestGradientIsBorrowedWhenTheSourceHasNone(t *testing.T) {
	authored := []TrackDef{{
		ID: "irohazaka", Name: "Stand-in", Source: SourceAuthored,
		RealWorld: "Iroha-zaka", Downhill: true, GradientMeasured: true,
		Segments: []Segment{
			{Name: "Top", Length: 100, Width: 7, Gradient: -4, Surface: SurfaceSmooth},
			{Name: "Middle", Length: 100, Width: 7, Gradient: -11, Surface: SurfaceSmooth},
			{Name: "Bottom", Length: 100, Width: 7, Gradient: -3, Surface: SurfaceSmooth},
		},
	}}
	// Measured corners, no elevation: every gradient reads zero.
	surveyed := []TrackDef{{
		ID: "irohazaka", Name: "Iroha-zaka", GradientMeasured: false,
		Segments: []Segment{
			{Name: "A", Radius: 18, Length: 60, Width: 7, Surface: SurfaceSmooth, Dir: -1},
			{Name: "B", Length: 120, Width: 7, Surface: SurfaceSmooth},
			{Name: "C", Radius: 22, Length: 60, Width: 7, Surface: SurfaceSmooth, Dir: 1},
			{Name: "D", Length: 60, Width: 7, Surface: SurfaceSmooth},
		},
	}}

	got := mergeSurveyed(authored, surveyed)[0]
	if got.Segments[0].Radius != 18 {
		t.Fatalf("measured geometry did not survive: R%.0f", got.Segments[0].Radius)
	}

	var steepest float64
	for _, seg := range got.Segments {
		if seg.Gradient == 0 {
			t.Errorf("%s came through flat — the borrow did not happen", seg.Name)
		}
		if seg.Gradient < steepest {
			steepest = seg.Gradient
		}
	}
	// The profile's character has to survive the remap: the steep middle is
	// still the steep bit.
	if steepest != -11 {
		t.Errorf("steepest borrowed gradient is %.0f%%, want the authored -11%%", steepest)
	}
	if d := got.Descent(); d <= 0 {
		t.Errorf("a borrowed descent still descends nothing: %.1fm", d)
	}

	// And a source that DID have elevation is left exactly as measured.
	measured := []TrackDef{{
		ID: "irohazaka", Name: "Iroha-zaka", GradientMeasured: true,
		Segments: []Segment{{Name: "A", Radius: 18, Length: 60, Width: 7, Gradient: -8.4, Surface: SurfaceSmooth}},
	}}
	if g := mergeSurveyed(authored, measured)[0].Segments[0].Gradient; g != -8.4 {
		t.Errorf("a measured gradient was overwritten: %.1f", g)
	}
}
