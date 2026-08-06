package game

import (
	"fmt"
	"math"
	"testing"
)

// referenceBuild is a stock car with a perfectly average driver: rating 100.
func referenceBuild() Build {
	return Build{
		Car: CarStats{
			PowerToWt: ReferencePowerToWt, Grip: ReferenceGrip,
			Handling: ReferenceHandling, Braking: ReferenceBraking,
			Drivetrain: FR,
		},
		Driver: DriverStats{
			Bravery: ReferenceDriverStat, Intelligence: ReferenceDriverStat,
			Reflexes: ReferenceDriverStat, Consistency: ReferenceDriverStat,
			Stamina: ReferenceDriverStat,
		},
	}
}

// scaledBuild scales every normalised input by m, which moves the rating by
// very close to m. Use edgeBuild when the exact figure matters.
func scaledBuild(m float64) Build {
	b := referenceBuild()
	b.Car.PowerToWt *= m
	b.Car.Grip *= m
	b.Car.Handling *= m
	b.Car.Braking *= m
	b.Driver.Bravery *= m
	b.Driver.Intelligence *= m
	b.Driver.Reflexes *= m
	b.Driver.Consistency *= m
	b.Driver.Stamina *= m
	return b
}

func winRate(t *testing.T, fav, dog Build, runs int) float64 {
	t.Helper()
	wins := 0
	for i := 0; i < runs; i++ {
		track := PickTrack(uint64(i))
		r := SimulateRace(track, fav, dog, uint64(i)*2654435761+1)
		if r.ChallWon {
			wins++
		}
	}
	return float64(wins) / float64(runs)
}

// edgeBuild returns a build rated exactly (1+pct) times the reference. The
// model is very nearly but not exactly linear — the stamina factor multiplies
// the linear terms — so we solve for the multiplier instead of assuming it.
// That way "a 5% rating advantage" means one precise thing in the tests below.
func edgeBuild(t *testing.T, pct float64) Build {
	t.Helper()
	target := OverallRating(referenceBuild()) * (1 + pct)
	lo, hi := 0.5, 3.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if OverallRating(scaledBuild(mid)) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	b := scaledBuild((lo + hi) / 2)
	if got := OverallRating(b) / OverallRating(referenceBuild()); math.Abs(got-(1+pct)) > 1e-6 {
		t.Fatalf("failed to solve for %+.0f%% edge, got %+.4f%%", pct*100, (got-1)*100)
	}
	return b
}

func TestRatingIsMonotonicAndNearLinear(t *testing.T) {
	base := OverallRating(referenceBuild())
	if math.Abs(base-100) > 1.5 {
		t.Fatalf("reference build should rate about 100, got %.4f", base)
	}
	prev := base
	for _, m := range []float64{1.05, 1.2, 1.5, 2.0} {
		got := OverallRating(scaledBuild(m))
		if got <= prev {
			t.Errorf("rating must increase with a better build: %.3f after %.3f", got, prev)
		}
		if rel := math.Abs(got-base*m) / (base * m); rel > 0.01 {
			t.Errorf("scaled(%.2f) rating = %.3f, want within 1%% of %.3f", m, got, base*m)
		}
		prev = got
	}
}

// TestFavoriteWinRate is the headline design requirement: the favourite in a
// realistic ladder matchup wins roughly 65% of the time.
func TestFavoriteWinRate(t *testing.T) {
	const runs = 200000
	rate := winRate(t, edgeBuild(t, 0.05), referenceBuild(), runs)
	if rate < 0.62 || rate > 0.68 {
		t.Errorf("favourite (+5%% rating) win rate = %.4f, want 0.62..0.68", rate)
	}
	t.Logf("+5%% rating favourite wins %.2f%% of %d races", rate*100, runs)
}

func TestEvenMatchIsCoinFlip(t *testing.T) {
	const runs = 200000
	rate := winRate(t, referenceBuild(), referenceBuild(), runs)
	if rate < 0.485 || rate > 0.515 {
		t.Errorf("identical builds win rate = %.4f, want ~0.50", rate)
	}
}

// A big enough gap should be close to decisive, but never a certainty — the
// mountain always gets a vote.
func TestBigAdvantageIsStrongNotCertain(t *testing.T) {
	const runs = 100000
	rate := winRate(t, edgeBuild(t, 0.35), referenceBuild(), runs)
	if rate < 0.95 {
		t.Errorf("+35%% rating win rate = %.4f, want >= 0.95", rate)
	}
	if rate > 0.9995 {
		t.Errorf("+35%% rating win rate = %.4f, upsets should still be possible", rate)
	}
}

// Within the range the ladder actually pairs people in, the underdog must
// keep a real chance. (Far outside it — a 2x build — the favourite is
// effectively certain, and that is intended.)
func TestUnderdogKeepsAChanceInLadderRange(t *testing.T) {
	const runs = 100000
	rate := winRate(t, referenceBuild(), edgeBuild(t, 0.15), runs)
	if rate < 0.08 {
		t.Errorf("underdog (-15%% rating) win rate = %.4f, want >= 0.08", rate)
	}
	t.Logf("underdog at -15%% rating still wins %.2f%%", rate*100)
}

func TestReplayIsDeterministic(t *testing.T) {
	a, b := scaledBuild(1.1), referenceBuild()
	for _, tr := range Tracks {
		r1 := SimulateRace(tr, a, b, 12345)
		r2 := SimulateRace(tr, a, b, 12345)
		if r1.ChallTotal != r2.ChallTotal || r1.OppTotal != r2.OppTotal {
			t.Fatalf("%s: replay diverged: %v vs %v", tr.ID, r1.ChallTotal, r2.ChallTotal)
		}
		for i := range r1.Segments {
			if r1.Segments[i] != r2.Segments[i] {
				t.Fatalf("%s segment %d diverged", tr.ID, i)
			}
		}
	}
}

func TestDifferentSeedsDifferentRaces(t *testing.T) {
	a, b := scaledBuild(1.1), referenceBuild()
	seen := map[float64]bool{}
	for s := uint64(0); s < 50; s++ {
		r := SimulateRace(Tracks[0], a, b, s)
		seen[r.ChallTotal] = true
	}
	if len(seen) < 45 {
		t.Errorf("expected distinct results per seed, got %d/50 unique", len(seen))
	}
}

func TestSegmentTotalsAddUp(t *testing.T) {
	r := SimulateRace(Tracks[0], scaledBuild(1.1), referenceBuild(), 99)
	var c, o float64
	for _, s := range r.Segments {
		c += s.ChallTime
		o += s.OppTime
	}
	if math.Abs(c-r.ChallTotal) > 0.01 || math.Abs(o-r.OppTotal) > 0.01 {
		t.Errorf("segment times don't sum to totals: %.3f/%.3f vs %.3f/%.3f", c, o, r.ChallTotal, r.OppTotal)
	}
	last := r.Segments[len(r.Segments)-1]
	if (last.Gap > 0) != r.ChallWon {
		t.Errorf("final gap sign %.3f disagrees with winner %v", last.Gap, r.ChallWon)
	}
}

// TestCalibrationCurve is documentation, not an assertion. Run with -v to see
// the whole advantage->win-rate curve after changing any tuning constant.
func TestCalibrationCurve(t *testing.T) {
	if testing.Short() {
		t.Skip("calibration curve skipped in short mode")
	}
	const runs = 60000
	fmt.Println("  rating edge   win rate")
	for _, m := range []float64{1.00, 1.02, 1.05, 1.10, 1.15, 1.25, 1.50, 2.00} {
		rate := winRate(t, scaledBuild(m), referenceBuild(), runs)
		fmt.Printf("  %+6.0f%%       %6.2f%%\n", (m-1)*100, rate*100)
	}
}

func TestMistakesHappenAtSaneRate(t *testing.T) {
	const runs = 20000
	b := referenceBuild()
	mistakes, segs := 0, 0
	for i := 0; i < runs; i++ {
		r := SimulateRace(Tracks[0], b, b, uint64(i))
		for _, s := range r.Segments {
			segs++
			if s.ChallMistake {
				mistakes++
			}
		}
	}
	rate := float64(mistakes) / float64(segs)
	if rate < 0.01 || rate > 0.08 {
		t.Errorf("per-segment mistake rate = %.4f, want 0.01..0.08", rate)
	}
	t.Logf("per-segment mistake rate %.2f%%", rate*100)
}

func TestConsistencyReducesVariance(t *testing.T) {
	const runs = 30000
	steady := referenceBuild()
	steady.Driver.Consistency = 30
	wild := referenceBuild()
	wild.Driver.Consistency = 4

	sd := func(b Build) float64 {
		var sum, sum2 float64
		for i := 0; i < runs; i++ {
			r := SimulateRace(Tracks[0], b, b, uint64(i))
			sum += r.ChallTotal
			sum2 += r.ChallTotal * r.ChallTotal
		}
		n := float64(runs)
		return math.Sqrt(sum2/n - (sum/n)*(sum/n))
	}
	if sd(steady) >= sd(wild) {
		t.Errorf("high consistency should produce tighter times: %.3f vs %.3f", sd(steady), sd(wild))
	}
}

// Close races should actually change hands on the way down the mountain. This
// is what FormMomentum buys; without it the run is decided at the start line.
func TestCloseRacesChangeLead(t *testing.T) {
	const runs = 20000
	fav, dog := edgeBuild(t, 0.03), referenceBuild()
	changed, close := 0, 0
	for i := 0; i < runs; i++ {
		r := SimulateRace(PickTrack(uint64(i)), fav, dog, uint64(i)*2654435761+1)
		if r.Margin > 3.0 {
			continue // a blowout has nothing to swap
		}
		close++
		if r.LeadChanges > 0 {
			changed++
		}
	}
	if close == 0 {
		t.Fatal("no close races generated")
	}
	rate := float64(changed) / float64(close)
	t.Logf("%.1f%% of close races (%d) changed lead at least once", rate*100, close)
	if rate < 0.5 {
		t.Errorf("only %.2f of close races changed lead; the corner-by-corner "+
			"presentation needs the gap to move", rate)
	}
}
