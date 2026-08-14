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
		best := map[int]int{}
		for _, track := range Tracks {
			bestV, bestScore := 0, math.Inf(-1)
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

	if got := LegalSetup(wound, UpgradeTiers{}); got != (Setup{}) {
		t.Errorf("stock car kept adjustments %+v", got)
	}

	// Coilovers but no arms and no dampers: ride height only.
	partial := LegalSetup(wound, UpgradeTiers{Suspension: 2})
	if partial.Ride != 2 {
		t.Errorf("ride height should be adjustable on coilovers, got %d", partial.Ride)
	}
	if partial.Camber != 0 || partial.Damping != 0 {
		t.Errorf("camber/damping adjustable without the parts: %+v", partial)
	}

	// Out-of-range values are pulled back rather than trusted.
	if got := LegalSetup(Setup{Gearing: 99}, allTiers(MaxTier)); got.Gearing != SetupMax {
		t.Errorf("gearing 99 survived as %d, want clamped to %d", got.Gearing, SetupMax)
	}
	if got := LegalSetup(Setup{Gearing: -99}, allTiers(MaxTier)); got.Gearing != SetupMin {
		t.Errorf("gearing -99 survived as %d, want clamped to %d", got.Gearing, SetupMin)
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
// what a determined player does by hand. Coordinate ascent rather than the
// full 5^7 cross product: it lands on the same answer for a model this smooth
// and is fast enough to run in a test.
func bestSetupFor(b Build, t TrackDef) Setup {
	best := Setup{}
	bestScore := TrackRating(WithSetup(b, best), t)
	for pass := 0; pass < 4; pass++ {
		improved := false
		for _, a := range SetupAxes {
			for v := SetupMin; v <= SetupMax; v++ {
				cand, _ := best.With(a.ID, v)
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
	return best
}

// The headline number for the whole feature: getting the car right for the
// road should be worth about a ladder rung, and no more. Too little and the
// spanner is decoration; too much and the garage stops mattering.
func TestPerfectSetupIsWorthAboutFivePercent(t *testing.T) {
	base := referenceBuild()
	var sum float64
	for _, track := range Tracks {
		factory := TrackRating(WithSetup(base, Setup{}), track)
		tuned := TrackRating(WithSetup(base, bestSetupFor(base, track)), track)
		gain := tuned/factory - 1
		sum += gain
		t.Logf("%-28s perfect setup worth %+.2f%%", track.Name, gain*100)
		if gain < 0.02 {
			t.Errorf("%s: a perfect setup is worth only %+.2f%% — not worth the screen", track.Name, gain*100)
		}
		if gain > 0.09 {
			t.Errorf("%s: a perfect setup is worth %+.2f%% — the sheet outweighs the build", track.Name, gain*100)
		}
	}
	if avg := sum / float64(len(Tracks)); avg < 0.035 || avg > 0.065 {
		t.Errorf("a perfect setup averages %+.2f%% across the game, want about 5%%", avg*100)
	} else {
		t.Logf("average across the game: %+.2f%%", avg*100)
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
	track, _ := TrackByID("akagi_down")

	layouts := []Drivetrain{FR, FF, MR, AWD}
	sheets := map[Drivetrain]Setup{}
	for _, dt := range layouts {
		b := referenceBuild()
		b.Car.Drivetrain = dt
		sheets[dt] = bestSetupFor(b, track)
	}
	distinct := map[Setup]bool{}
	for _, s := range sheets {
		distinct[s] = true
	}
	if len(distinct) < 2 {
		t.Errorf("every layout wants the identical sheet on %s: %+v", track.Name, sheets[FR])
	}

	// A powerful car and a nimble one should also disagree.
	powerful := referenceBuild()
	powerful.Car.PowerToWt *= 1.6
	nimble := referenceBuild()
	nimble.Car.Grip *= 1.4
	nimble.Car.Handling *= 1.3
	if bestSetupFor(powerful, track) == bestSetupFor(nimble, track) {
		t.Error("a power car and a grip car want the identical sheet")
	}
}
