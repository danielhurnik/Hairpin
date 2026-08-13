package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// Buying a car is a currency path, so it gets the same treatment as the rest
// of the economy: the client names a chassis and nothing else, and every way
// of getting one for free is tried here explicitly.

// give sets a player's cash and level directly, so a test can stand at the
// showroom counter without grinding jobs to get there.
func give(t *testing.T, st *Store, playerID string, cash int64, level int) {
	t.Helper()
	if _, err := st.DB().Exec(`UPDATE players SET cash = ?, driver_level = ? WHERE id = ?`,
		cash, level, playerID); err != nil {
		t.Fatalf("give: %v", err)
	}
}

func mustBuy(t *testing.T, st *Store, playerID, chassisID string) *Player {
	t.Helper()
	p, _, err := st.BuyChassis(context.Background(), playerID, chassisID)
	if err != nil {
		t.Fatalf("buy %s: %v", chassisID, err)
	}
	return p
}

func TestBuyingACarChargesOnceAndArrivesStock(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	def, _ := game.ChassisByID("eg_hatch")
	give(t, st, p.ID, def.Price+5000, def.MinLevel)

	after, paid, err := st.BuyChassis(ctx, p.ID, "eg_hatch")
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if paid != def.Price {
		t.Fatalf("paid %d, want the table price %d", paid, def.Price)
	}
	if after.Cash != 5000 {
		t.Fatalf("cash %d after buying, want 5000", after.Cash)
	}
	// Buying does not put you in the car — that is a separate, free choice.
	if after.ChassisID != game.StarterChassis {
		t.Fatalf("active car changed to %s on purchase", after.ChassisID)
	}

	owned, err := ownedCars(ctx, st.DB(), p.ID)
	if err != nil {
		t.Fatalf("owned: %v", err)
	}
	if len(owned) != 2 {
		t.Fatalf("owns %d cars, want starter plus the new one", len(owned))
	}
	if tiers := owned["eg_hatch"]; tiers != (game.UpgradeTiers{}) {
		t.Fatalf("new car arrived with tiers %+v, want stock", tiers)
	}
}

func TestCarCannotBeBoughtTwice(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	def, _ := game.ChassisByID("eg_hatch")
	give(t, st, p.ID, def.Price*3, def.MinLevel)

	mustBuy(t, st, p.ID, "eg_hatch")
	before, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if _, _, err := st.BuyChassis(ctx, p.ID, "eg_hatch"); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("second purchase returned %v, want ErrAlreadyOwned", err)
	}
	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// The rejected purchase must not have taken the money with it.
	if after.Cash != before.Cash {
		t.Fatalf("cash went %d -> %d on a rejected purchase", before.Cash, after.Cash)
	}
}

func TestCarPurchaseRespectsCashAndLevel(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	def, _ := game.ChassisByID("gc_awd")

	// Rich enough, too junior.
	give(t, st, p.ID, def.Price*2, def.MinLevel-1)
	if _, _, err := st.BuyChassis(ctx, p.ID, "gc_awd"); !errors.Is(err, ErrLevelLocked) {
		t.Fatalf("under-level purchase returned %v, want ErrLevelLocked", err)
	}

	// Senior enough, too poor.
	give(t, st, p.ID, def.Price-1, def.MinLevel)
	if _, _, err := st.BuyChassis(ctx, p.ID, "gc_awd"); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("underfunded purchase returned %v, want ErrInsufficient", err)
	}

	// One yen more and it goes through.
	give(t, st, p.ID, def.Price, def.MinLevel)
	mustBuy(t, st, p.ID, "gc_awd")
}

func TestStarterCarIsNotForSale(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	give(t, st, p.ID, 1_000_000, 50)
	// The starter is owned from the first login; selling it again would be a
	// free way to reset a car's tiers to stock.
	if _, _, err := st.BuyChassis(context.Background(), p.ID, game.StarterChassis); !errors.Is(err, ErrInvalid) {
		t.Fatalf("buying the starter returned %v, want ErrInvalid", err)
	}
}

func TestCannotDriveACarYouDoNotOwn(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	if _, err := st.SetActiveCar(context.Background(), p.ID, "r_gt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("equipping an unowned car returned %v, want ErrNotFound", err)
	}
	after, err := st.GetPlayer(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.ChassisID != game.StarterChassis {
		t.Fatalf("active car became %s", after.ChassisID)
	}
}

// The load-bearing rule of the whole feature: tiers belong to a car, not to a
// player. If they leaked, buying a car would hand over a finished build.
func TestUpgradesDoNotLeakBetweenCars(t *testing.T) {
	st, _, p := newTestStore(t, 4)
	ctx := context.Background()
	give(t, st, p.ID, 500_000, 30)

	// Build up the starter.
	for i := 0; i < 3; i++ {
		if _, _, err := st.BuyUpgrade(ctx, p.ID, game.CatEngine); err != nil {
			t.Fatalf("upgrade starter: %v", err)
		}
	}
	built, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if built.Tiers.Engine != 3 {
		t.Fatalf("starter engine tier %d, want 3", built.Tiers.Engine)
	}

	// Buy a second car and drive it.
	mustBuy(t, st, p.ID, "eg_hatch")
	switched, err := st.SetActiveCar(ctx, p.ID, "eg_hatch")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if switched.Tiers.Engine != 0 {
		t.Fatalf("new car reads engine tier %d, want a stock 0", switched.Tiers.Engine)
	}

	// Upgrading now must land on the new car only.
	if _, _, err := st.BuyUpgrade(ctx, p.ID, game.CatEngine); err != nil {
		t.Fatalf("upgrade new car: %v", err)
	}
	owned, err := ownedCars(ctx, st.DB(), p.ID)
	if err != nil {
		t.Fatalf("owned: %v", err)
	}
	if owned["eg_hatch"].Engine != 1 {
		t.Fatalf("new car engine tier %d, want 1", owned["eg_hatch"].Engine)
	}
	if owned[game.StarterChassis].Engine != 3 {
		t.Fatalf("starter engine tier %d, want its own 3 untouched", owned[game.StarterChassis].Engine)
	}

	// And switching back restores the original build intact.
	back, err := st.SetActiveCar(ctx, p.ID, game.StarterChassis)
	if err != nil {
		t.Fatalf("switch back: %v", err)
	}
	if back.Tiers.Engine != 3 {
		t.Fatalf("returning to the starter reads engine tier %d, want 3", back.Tiers.Engine)
	}
}

// Firing the same purchase twelve times at once is what a cheating client
// actually sends. Exactly one may land, and the player pays exactly once.
func TestConcurrentCarPurchasesBuyOneCar(t *testing.T) {
	st, _, p := newTestStore(t, 3)
	ctx := context.Background()
	def, _ := game.ChassisByID("eg_hatch")
	give(t, st, p.ID, def.Price, def.MinLevel)

	ok, errs := fanOut(12, func() error {
		_, _, err := st.BuyChassis(ctx, p.ID, "eg_hatch")
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 12 simultaneous purchases succeeded, want exactly 1", ok)
	}
	for _, err := range errs {
		if !errors.Is(err, ErrInsufficient) && !errors.Is(err, ErrAlreadyOwned) {
			t.Errorf("unexpected error from a losing request: %v", err)
		}
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if after.Cash != 0 {
		t.Errorf("cash = %d, want 0 — the player paid for more cars than they got", after.Cash)
	}
	owned, err := ownedCars(ctx, st.DB(), p.ID)
	if err != nil {
		t.Fatalf("owned: %v", err)
	}
	if len(owned) != 2 {
		t.Errorf("owns %d cars, want the starter plus exactly one purchase", len(owned))
	}
}

// ---------------------------------------------------------------------------
// Migration
// ---------------------------------------------------------------------------

// oldPlayersDDL is the players table as it stood when every player had exactly
// one implicit car, with the upgrade tiers as columns.
const oldPlayersDDL = `
CREATE TABLE players (
    id             TEXT PRIMARY KEY,
    device_hash    TEXT UNIQUE,
    display_name   TEXT NOT NULL,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    cash           INTEGER NOT NULL DEFAULT 0,
    rubies         INTEGER NOT NULL DEFAULT 0,
    driver_level   INTEGER NOT NULL DEFAULT 1,
    driver_xp      INTEGER NOT NULL DEFAULT 0,
    unspent_points INTEGER NOT NULL DEFAULT 0,
    stat_bravery      REAL NOT NULL DEFAULT 10,
    stat_intelligence REAL NOT NULL DEFAULT 10,
    stat_reflexes     REAL NOT NULL DEFAULT 10,
    stat_consistency  REAL NOT NULL DEFAULT 10,
    stat_stamina      REAL NOT NULL DEFAULT 10,
    chassis_id     TEXT NOT NULL DEFAULT 'ae_coupe',
    tier_engine     INTEGER NOT NULL DEFAULT 0,
    tier_turbo      INTEGER NOT NULL DEFAULT 0,
    tier_tires      INTEGER NOT NULL DEFAULT 0,
    tier_suspension INTEGER NOT NULL DEFAULT 0,
    tier_weight     INTEGER NOT NULL DEFAULT 0,
    livery_id      TEXT NOT NULL DEFAULT 'panda',
    ladder_rank    INTEGER NOT NULL,
    wins           INTEGER NOT NULL DEFAULT 0,
    losses         INTEGER NOT NULL DEFAULT 0,
    free_races       INTEGER NOT NULL DEFAULT 5,
    extra_races_used INTEGER NOT NULL DEFAULT 0,
    races_reset_at   INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);`

// A player who spent real time building a car must find that car intact after
// the upgrade tiers move tables underneath them.
func TestMigrationCarriesExistingBuildOntoACar(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "legacy.db") + "?_pragma=foreign_keys(1)"

	// Stand up a database in the shape the old schema left behind.
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(oldPlayersDDL); err != nil {
		t.Fatalf("old ddl: %v", err)
	}
	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC).Unix()
	if _, err := raw.Exec(`INSERT INTO players
		(id, device_hash, display_name, cash, chassis_id, tier_engine, tier_turbo, tier_tires,
		 tier_suspension, tier_weight, livery_id, ladder_rank, created_at, updated_at)
		VALUES ('veteran', 'legacy-device', 'NightFox', 9000, 'fc_rotary', 5, 4, 3, 2, 1,
		        'panda', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed legacy player: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	// Opening the store is what migrates it.
	st, err := Open(dsn, Options{TimeScale: 1})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	p, err := st.GetPlayer(context.Background(), "veteran")
	if err != nil {
		t.Fatalf("get migrated player: %v", err)
	}
	want := game.UpgradeTiers{Engine: 5, Turbo: 4, Tires: 3, Suspension: 2, Weight: 1}
	if p.Tiers != want {
		t.Fatalf("migrated tiers %+v, want %+v", p.Tiers, want)
	}
	if p.ChassisID != "fc_rotary" {
		t.Fatalf("migrated chassis %s, want fc_rotary", p.ChassisID)
	}
	if p.Cash != 9000 {
		t.Fatalf("migrated cash %d, want 9000", p.Cash)
	}

	// The old columns are gone, so there is no second place to read a tier from.
	if had, err := hasColumn(st.DB(), "players", "tier_engine"); err != nil || had {
		t.Fatalf("tier_engine still on players (err %v)", err)
	}

	// The migrated car is a real owned car: it can be upgraded and raced.
	if _, _, err := st.BuyUpgrade(context.Background(), "veteran", game.CatTires); err != nil {
		t.Fatalf("upgrade after migration: %v", err)
	}
}

// Reopening a migrated database must not run the migration a second time or
// reset anyone to stock — dev.sh reopens the same file constantly.
func TestMigrationIsIdempotent(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "reopen.db") + "?_pragma=foreign_keys(1)"
	ctx := context.Background()

	st, err := Open(dsn, Options{TimeScale: 1})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	p, _, err := st.EnsurePlayer(ctx, "reopen-device")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	give(t, st, p.ID, 500_000, 30)
	if _, _, err := st.BuyUpgrade(ctx, p.ID, game.CatTurbo); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	mustBuy(t, st, p.ID, "eg_hatch")
	st.Close()

	reopened, err := Open(dsn, Options{TimeScale: 1})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	after, err := reopened.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Tiers.Turbo != 1 {
		t.Fatalf("turbo tier %d after reopen, want 1", after.Tiers.Turbo)
	}
	owned, err := ownedCars(ctx, reopened.DB(), p.ID)
	if err != nil {
		t.Fatalf("owned: %v", err)
	}
	if len(owned) != 2 {
		t.Fatalf("owns %d cars after reopen, want 2", len(owned))
	}
}
