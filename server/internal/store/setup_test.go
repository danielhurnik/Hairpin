package store

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// buildUp raises every category on the active car to tier, so a test can reach
// the adjustments that part unlocks.
func buildUp(t *testing.T, st *Store, playerID string, tier int) {
	t.Helper()
	give(t, st, playerID, 5_000_000, 50)
	ctx := context.Background()
	for _, cat := range game.UpgradeCategories {
		for i := 0; i < tier; i++ {
			if _, _, err := st.BuyUpgrade(ctx, playerID, cat); err != nil {
				t.Fatalf("upgrade %s: %v", cat, err)
			}
		}
	}
}

func TestSetupNeedsThePartFitted(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()

	// A stock car has nothing to turn.
	for _, a := range game.SetupAxes {
		if _, err := st.SetSetup(ctx, p.ID, a.ID, a.Setting(1)); !errors.Is(err, ErrPartMissing) {
			t.Errorf("adjusting %q on a stock car returned %v, want ErrPartMissing", a.ID, err)
		}
	}

	// Coilovers arrive at suspension tier 2 and bring ride height with them.
	buildUp(t, st, p.ID, 2)
	if _, err := st.SetSetup(ctx, p.ID, "ride_height", 87); err != nil {
		t.Fatalf("ride height on coilovers: %v", err)
	}
	// Camber still needs the arms.
	if _, err := st.SetSetup(ctx, p.ID, "camber", -2.8); !errors.Is(err, ErrPartMissing) {
		t.Errorf("camber without pillow-ball arms returned %v, want ErrPartMissing", err)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	ride, _ := game.AxisByID("ride_height")
	if got := ride.Setting(after.Setup.Ride); got != 87 {
		t.Errorf("ride height stored as %.2fmm, want the 87mm that was asked for", got)
	}
}

// The whole point of the sheet being physical: a player sets a number, not one
// of five positions, and gets back exactly the number they set.
func TestSetupTakesRealNumbers(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	// Every axis, at a value that is deliberately not one of the old notches.
	for _, a := range game.SetupAxes {
		between := (a.Setting(0) + a.Setting(1)) / 2
		want := a.Factory() + roundStep(between-a.Factory(), a.Step)
		if _, err := st.SetSetup(ctx, p.ID, a.ID, between); err != nil {
			t.Fatalf("set %s to %v: %v", a.ID, between, err)
		}
		after, err := st.GetPlayer(ctx, p.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		v, _ := after.Setup.Get(a.ID)
		if got := a.Setting(v); math.Abs(got-want) > a.Step/2 {
			t.Errorf("%s set to %v came back as %v, want %v", a.ID, between, got, want)
		}
	}

	// And the workshop's own precision is enforced: nothing finer than a step
	// survives, because nothing finer than a step can be set on the car.
	ride, _ := game.AxisByID("ride_height")
	if _, err := st.SetSetup(ctx, p.ID, "ride_height", 87.4); err != nil {
		t.Fatalf("set ride height: %v", err)
	}
	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := ride.Setting(after.Setup.Ride); got != 87 {
		t.Errorf("87.4mm stored as %vmm, want it snapped to the millimetre", got)
	}
}

func roundStep(v, step float64) float64 {
	return math.Round(v/step) * step
}

func TestSetupRejectsValuesOffTheSheet(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	// Final drives the workshop does not have gears for.
	for _, v := range []float64{5.2, 2.9, 99, -99, math.NaN(), math.Inf(1)} {
		if _, err := st.SetSetup(ctx, p.ID, "gearing", v); !errors.Is(err, ErrInvalid) {
			t.Errorf("gearing %v returned %v, want ErrInvalid", v, err)
		}
	}
	// Positions off the ends of the axis, for a client that sends those.
	for _, v := range []float64{game.SetupMax + 0.01, game.SetupMin - 0.01, 99, math.NaN()} {
		if _, err := st.SetSetupPosition(ctx, p.ID, "gearing", v); !errors.Is(err, ErrInvalid) {
			t.Errorf("position %v returned %v, want ErrInvalid", v, err)
		}
	}
	if _, err := st.SetSetup(ctx, p.ID, "nitrous", 1); !errors.Is(err, ErrInvalid) {
		t.Error("an axis that does not exist was accepted")
	}
}

// A setup belongs to a car, like the parts it adjusts.
func TestSetupsArePerCar(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	if _, err := st.SetSetup(ctx, p.ID, "gearing", 3.70); err != nil {
		t.Fatalf("set gearing: %v", err)
	}

	give(t, st, p.ID, 5_000_000, 50)
	mustBuy(t, st, p.ID, "eg_hatch")
	switched, err := st.SetActiveCar(ctx, p.ID, "eg_hatch")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if switched.Setup.Gearing != 0 {
		t.Errorf("the new car came with gearing %v, want a factory 0", switched.Setup.Gearing)
	}

	back, err := st.SetActiveCar(ctx, p.ID, game.StarterChassis)
	if err != nil {
		t.Fatalf("switch back: %v", err)
	}
	gearing, _ := game.AxisByID("gearing")
	if got := gearing.Setting(back.Setup.Gearing); got != 3.70 {
		t.Errorf("returning to the first car reads a %.2f final drive, want the 3.70 it was left on", got)
	}
}

// Tuning is free — the money went on the parts.
func TestAdjustingASetupCostsNothing(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	before, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := st.SetSetup(ctx, p.ID, "camber", -1.0-float64(i)*0.1); err != nil {
			t.Fatalf("adjust: %v", err)
		}
	}
	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Cash != before.Cash || after.Rubies != before.Rubies {
		t.Errorf("twenty adjustments cost %d cash and %d rubies, want nothing",
			before.Cash-after.Cash, before.Rubies-after.Rubies)
	}
}

func TestResetSetupReturnsTheCarToFactory(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	for _, a := range game.SetupAxes {
		if _, err := st.SetSetup(ctx, p.ID, a.ID, a.SettingHigh()); err != nil {
			t.Fatalf("set %s: %v", a.ID, err)
		}
	}
	after, err := st.ResetSetup(ctx, p.ID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !after.Setup.IsFactory() {
		t.Errorf("reset left %+v behind", after.Setup)
	}
}

// The sheet must describe the road ahead, and lock what has not been bought.
func TestSetupViewExplainsWhatIsLocked(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()

	view := st.SetupFor(ctx, p)
	if view.AnyUnlocked {
		t.Error("a stock car reports something adjustable")
	}
	if view.Track.ID == "" {
		t.Error("the sheet does not say which road it is for")
	}
	for _, a := range view.Axes {
		if a.Unlocked {
			t.Errorf("axis %q unlocked on a stock car", a.ID)
		}
		if a.Requires == "" {
			t.Errorf("locked axis %q does not say what to buy", a.ID)
		}
	}

	buildUp(t, st, p.ID, game.MaxTier)
	built, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	full := st.SetupFor(ctx, built)
	if !full.AnyUnlocked {
		t.Fatal("a fully built car reports nothing adjustable")
	}
	for _, a := range full.Axes {
		if !a.Unlocked {
			t.Errorf("axis %q still locked on a fully built car", a.ID)
		}
		// The sheet states the setting and takes no view on whether it is the
		// right one. A number, a unit, and no score.
		if a.Display == "" {
			t.Errorf("axis %q does not say what it is set to", a.ID)
		}
		if a.Setting != a.SetupAxis.Setting(a.Value) {
			t.Errorf("axis %q reports %v but position %v is %v",
				a.ID, a.Setting, a.Value, a.SetupAxis.Setting(a.Value))
		}
		// A client needs the ends of the range to offer them, and the step to
		// know how finely they can be asked for.
		if a.SettingLow == a.SettingHigh || a.Step <= 0 {
			t.Errorf("axis %q publishes range %v..%v step %v — nothing continuous can be built on that",
				a.ID, a.SettingLow, a.SettingHigh, a.Step)
		}
	}
}

// Whatever the sheet says, the overall rating must not move — the garage
// number stays honest and only the per-track figure responds.
func TestTuningDoesNotMoveTheGarageRating(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	built, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	base, _, err := st.BuildFor(ctx, built)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	baseRating := game.OverallRating(base)

	for _, a := range game.SetupAxes {
		if _, err := st.SetSetup(ctx, p.ID, a.ID, a.SettingHigh()); err != nil {
			t.Fatalf("set %s: %v", a.ID, err)
		}
	}
	tuned, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	b2, _, err := st.BuildFor(ctx, tuned)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := game.OverallRating(b2); got-baseRating > 0.05 || baseRating-got > 0.05 {
		t.Errorf("overall rating moved %.4f -> %.4f on a fully wound setup", baseRating, got)
	}
}

// Rivals have to turn up with the car set up for something. Racing a ladder
// full of factory sheets hands anyone who tunes a free edge on every single
// race — and tuning is worth about 5%, so it is not a small gift.
func TestBotsArriveWithSetupsOfTheirOwn(t *testing.T) {
	st, _, _ := newTestStore(t, 60)
	ctx := context.Background()

	rows, err := st.DB().QueryContext(ctx, `SELECT c.setup_gearing, c.setup_boost, c.setup_pressure,
		c.setup_ballast, c.setup_ride, c.setup_camber, c.setup_damping, p.ladder_rank
		FROM player_cars c JOIN players p ON p.id = c.player_id WHERE p.is_bot = 1
		ORDER BY p.ladder_rank`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var tunedTop, tunedBottom, total, top int
	for rows.Next() {
		var s game.Setup
		var rank int
		if err := rows.Scan(&s.Gearing, &s.Boost, &s.Pressure, &s.Ballast,
			&s.Ride, &s.Camber, &s.Damping, &rank); err != nil {
			t.Fatalf("scan: %v", err)
		}
		total++
		if !s.IsFactory() {
			if rank <= 20 {
				tunedTop++
			} else {
				tunedBottom++
			}
		}
		if rank <= 20 {
			top++
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if total == 0 {
		t.Fatal("no bots seeded")
	}
	// The top of the mountain knows what it is doing.
	if tunedTop < top*3/4 {
		t.Errorf("only %d of the top %d rivals had touched the car", tunedTop, top)
	}
	// The bottom mostly has not, which is what makes climbing feel like
	// climbing rather than like meeting the same driver twenty times.
	if tunedBottom >= total-top {
		t.Errorf("every one of the %d lower rivals is tuned; nobody is a novice",
			total-top)
	}
}

// A bot cannot adjust a part it never bought, exactly like a player.
func TestBotSetupsRespectTheirOwnParts(t *testing.T) {
	st, _, _ := newTestStore(t, 60)
	ctx := context.Background()

	rows, err := st.DB().QueryContext(ctx, `SELECT tier_engine, tier_turbo, tier_tires,
		tier_suspension, tier_weight, setup_gearing, setup_boost, setup_pressure,
		setup_ballast, setup_ride, setup_camber, setup_damping
		FROM player_cars c JOIN players p ON p.id = c.player_id WHERE p.is_bot = 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tiers game.UpgradeTiers
		var s game.Setup
		if err := rows.Scan(&tiers.Engine, &tiers.Turbo, &tiers.Tires, &tiers.Suspension,
			&tiers.Weight, &s.Gearing, &s.Boost, &s.Pressure, &s.Ballast,
			&s.Ride, &s.Camber, &s.Damping); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if legal := game.LegalSetup(s, tiers); legal != s {
			t.Fatalf("a bot is running %+v on tiers %+v, which strips to %+v", s, tiers, legal)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
