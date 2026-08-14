package store

import (
	"context"
	"errors"
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
		if _, err := st.SetSetup(ctx, p.ID, a.ID, 1); !errors.Is(err, ErrPartMissing) {
			t.Errorf("adjusting %q on a stock car returned %v, want ErrPartMissing", a.ID, err)
		}
	}

	// Coilovers arrive at suspension tier 2 and bring ride height with them.
	buildUp(t, st, p.ID, 2)
	if _, err := st.SetSetup(ctx, p.ID, "ride_height", -2); err != nil {
		t.Fatalf("ride height on coilovers: %v", err)
	}
	// Camber still needs the arms.
	if _, err := st.SetSetup(ctx, p.ID, "camber", 1); !errors.Is(err, ErrPartMissing) {
		t.Errorf("camber without pillow-ball arms returned %v, want ErrPartMissing", err)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Setup.Ride != -2 {
		t.Errorf("ride height stored as %d, want -2", after.Setup.Ride)
	}
}

func TestSetupRejectsValuesOffTheSheet(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	buildUp(t, st, p.ID, game.MaxTier)

	for _, v := range []int{game.SetupMax + 1, game.SetupMin - 1, 99, -99} {
		if _, err := st.SetSetup(ctx, p.ID, "gearing", v); !errors.Is(err, ErrInvalid) {
			t.Errorf("gearing %d returned %v, want ErrInvalid", v, err)
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

	if _, err := st.SetSetup(ctx, p.ID, "gearing", -2); err != nil {
		t.Fatalf("set gearing: %v", err)
	}

	give(t, st, p.ID, 5_000_000, 50)
	mustBuy(t, st, p.ID, "eg_hatch")
	switched, err := st.SetActiveCar(ctx, p.ID, "eg_hatch")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if switched.Setup.Gearing != 0 {
		t.Errorf("the new car came with gearing %d, want a factory 0", switched.Setup.Gearing)
	}

	back, err := st.SetActiveCar(ctx, p.ID, game.StarterChassis)
	if err != nil {
		t.Fatalf("switch back: %v", err)
	}
	if back.Setup.Gearing != -2 {
		t.Errorf("returning to the first car reads gearing %d, want -2", back.Setup.Gearing)
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
		if _, err := st.SetSetup(ctx, p.ID, "camber", i%3-1); err != nil {
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
		if _, err := st.SetSetup(ctx, p.ID, a.ID, 2); err != nil {
			t.Fatalf("set %s: %v", a.ID, err)
		}
	}
	after, err := st.ResetSetup(ctx, p.ID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if after.Setup != (game.Setup{}) {
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
			t.Errorf("axis %q reports %v but its notch %d is %v",
				a.ID, a.Setting, a.Value, a.SetupAxis.Setting(a.Value))
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
		if _, err := st.SetSetup(ctx, p.ID, a.ID, 2); err != nil {
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
		if s != (game.Setup{}) {
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
