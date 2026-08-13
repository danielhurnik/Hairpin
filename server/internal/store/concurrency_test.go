package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// The economy's integrity rests on database constraints rather than on
// request timing. These tests fire the duplicate requests a cheating client
// would actually send — simultaneously — and check that exactly one wins.

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestStore(t *testing.T, bots int) (*Store, *testClock, *Player) {
	t.Helper()
	clk := &testClock{t: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)}
	dsn := "file:" + filepath.Join(t.TempDir(), "concurrency.db") + "?_pragma=foreign_keys(1)"
	st, err := Open(dsn, Options{TimeScale: 1, Now: clk.now})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if _, err := st.SeedBots(context.Background(), bots); err != nil {
		t.Fatalf("seed bots: %v", err)
	}
	p, _, err := st.EnsurePlayer(context.Background(), "device-hash-under-test")
	if err != nil {
		t.Fatalf("ensure player: %v", err)
	}
	return st, clk, p
}

// fanOut runs fn n times at once and returns how many returned nil.
func fanOut(n int, fn func() error) (successes int, errs []error) {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		start = make(chan struct{})
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // line everyone up before releasing
			err := fn()
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				errs = append(errs, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	return successes, errs
}

func TestConcurrentClaimPaysOnce(t *testing.T) {
	st, clk, p := newTestStore(t, 5)
	ctx := context.Background()

	if _, err := st.StartJob(ctx, p.ID, "tofu_run"); err != nil {
		t.Fatalf("start job: %v", err)
	}
	clk.advance(2 * time.Minute)

	before, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}

	ok, _ := fanOut(16, func() error {
		_, err := st.ClaimJob(ctx, p.ID)
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 16 simultaneous claims succeeded, want exactly 1", ok)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if got := after.Cash - before.Cash; got != 45 {
		t.Errorf("cash increased by %d, want exactly one payout of 45", got)
	}
}

func TestConcurrentJobStartsCreateOneJob(t *testing.T) {
	st, _, p := newTestStore(t, 5)
	ctx := context.Background()

	ok, _ := fanOut(12, func() error {
		_, err := st.StartJob(ctx, p.ID, "tofu_run")
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 12 simultaneous starts succeeded, want exactly 1", ok)
	}

	var active int
	if err := st.db.QueryRow(
		`SELECT COUNT(*) FROM player_jobs WHERE player_id = ? AND claimed_at IS NULL`, p.ID,
	).Scan(&active); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if active != 1 {
		t.Errorf("%d unclaimed jobs on record, want 1", active)
	}
}

func TestConcurrentRacesRespectDailyAllowance(t *testing.T) {
	st, _, p := newTestStore(t, 30)
	ctx := context.Background()

	// 5 free races + 50 starting rubies at 15 each = 3 paid = 8 total.
	const want = game.FreeRacesPerDay + 3
	ok, _ := fanOut(40, func() error {
		_, err := st.RunRace(ctx, p.ID)
		return err
	})
	if ok != want {
		t.Errorf("%d of 40 simultaneous races succeeded, want exactly %d", ok, want)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if after.Rubies < 0 {
		t.Errorf("rubies went negative: %d", after.Rubies)
	}
	if after.Wins+after.Losses != want {
		t.Errorf("recorded %d races, want %d", after.Wins+after.Losses, want)
	}
	if after.Rubies != StartingRubies-3*game.ExtraRaceRubies {
		t.Errorf("rubies = %d, want %d", after.Rubies, StartingRubies-3*game.ExtraRaceRubies)
	}
}

func TestConcurrentShopBuysTakeOneOffer(t *testing.T) {
	st, _, p := newTestStore(t, 5)
	ctx := context.Background()

	// Give the player enough cash that only the offer limit can stop them.
	if _, err := st.db.Exec(`UPDATE players SET cash = 100000000 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("grant cash: %v", err)
	}
	fresh, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	view, err := st.Shop(ctx, fresh)
	if err != nil {
		t.Fatalf("shop: %v", err)
	}
	if len(view.Offers) == 0 {
		t.Fatal("shop is empty")
	}
	offer := view.Offers[0]

	ok, _ := fanOut(16, func() error {
		_, _, err := st.BuyOffer(ctx, p.ID, offer.OfferID)
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 16 simultaneous purchases succeeded, want exactly 1", ok)
	}

	var owned int
	if err := st.db.QueryRow(
		`SELECT COUNT(*) FROM inventory WHERE player_id = ? AND item_id = ?`, p.ID, offer.ItemID,
	).Scan(&owned); err != nil {
		t.Fatalf("count inventory: %v", err)
	}
	if owned != 1 {
		t.Errorf("player owns %d copies, want 1", owned)
	}
}

func TestConcurrentReceiptRedemptionCreditsOnce(t *testing.T) {
	st, _, p := newTestStore(t, 3)
	ctx := context.Background()

	ok, _ := fanOut(16, func() error {
		_, _, err := st.GrantRubies(ctx, p.ID, "pack_case", "one-and-only-receipt")
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 16 redemptions of one receipt succeeded, want exactly 1", ok)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if want := int64(StartingRubies + 1400); after.Rubies != want {
		t.Errorf("rubies = %d, want %d (one credit of 1200+200)", after.Rubies, want)
	}
}

// A player with exactly enough cash for one upgrade must not get two.
func TestConcurrentUpgradesCannotOverspend(t *testing.T) {
	st, _, p := newTestStore(t, 3)
	ctx := context.Background()

	cost, _ := game.UpgradeCost(game.CatTires, 1)
	if _, err := st.db.Exec(`UPDATE players SET cash = ? WHERE id = ?`, cost, p.ID); err != nil {
		t.Fatalf("set cash: %v", err)
	}

	ok, errs := fanOut(12, func() error {
		_, _, err := st.BuyUpgrade(ctx, p.ID, game.CatTires)
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 12 simultaneous upgrades succeeded, want exactly 1", ok)
	}
	for _, err := range errs {
		if !errors.Is(err, ErrInsufficient) {
			t.Errorf("unexpected error from a losing request: %v", err)
		}
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if after.Cash != 0 {
		t.Errorf("cash = %d, want 0 — the player paid for more than they bought", after.Cash)
	}
	if after.Tiers.Tires != 1 {
		t.Errorf("tires tier = %d, want 1", after.Tiers.Tires)
	}
}

// Stat points must not be spendable twice.
func TestConcurrentStatAllocationSpendsOnce(t *testing.T) {
	st, _, p := newTestStore(t, 3)
	ctx := context.Background()

	if _, err := st.db.Exec(`UPDATE players SET unspent_points = 1 WHERE id = ?`, p.ID); err != nil {
		t.Fatalf("grant point: %v", err)
	}

	ok, _ := fanOut(12, func() error {
		_, err := st.AllocateStat(ctx, p.ID, "bravery", 1)
		return err
	})
	if ok != 1 {
		t.Errorf("%d of 12 simultaneous allocations succeeded, want exactly 1", ok)
	}

	after, err := st.GetPlayer(ctx, p.ID)
	if err != nil {
		t.Fatalf("get player: %v", err)
	}
	if after.UnspentPoints != 0 {
		t.Errorf("unspent points = %d, want 0", after.UnspentPoints)
	}
	if want := float64(game.StartingStatValue + 1); after.Stats.Bravery != want {
		t.Errorf("bravery = %.0f, want %.0f", after.Stats.Bravery, want)
	}
}

// Equipping several items into the same slot concurrently must leave exactly
// one equipped — the partial unique index is what guarantees it.
func TestConcurrentEquipKeepsOnePerSlot(t *testing.T) {
	st, _, p := newTestStore(t, 3)
	ctx := context.Background()

	var ids []int64
	for i := 0; i < 6; i++ {
		item, err := insertItem(ctx, st.db, p.ID,
			game.Drop{ItemID: "cotton_gloves", Quality: 1}, "test", st.Now())
		if err != nil {
			t.Fatalf("insert item: %v", err)
		}
		ids = append(ids, item.ID)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_ = st.Equip(ctx, p.ID, id)
		}(id)
	}
	wg.Wait()

	var equipped int
	if err := st.db.QueryRow(
		`SELECT COUNT(*) FROM inventory WHERE player_id = ? AND slot = 'hands' AND equipped = 1`, p.ID,
	).Scan(&equipped); err != nil {
		t.Fatalf("count equipped: %v", err)
	}
	if equipped != 1 {
		t.Errorf("%d items equipped in the hands slot, want 1", equipped)
	}
}

// The ladder must remain a permutation of 1..N no matter how the races land.
func TestLadderStaysAPermutation(t *testing.T) {
	st, clk, p := newTestStore(t, 40)
	ctx := context.Background()

	for day := 0; day < 15; day++ {
		for i := 0; i < game.FreeRacesPerDay; i++ {
			if _, err := st.RunRace(ctx, p.ID); err != nil && !errors.Is(err, ErrNoRaces) {
				t.Fatalf("race: %v", err)
			}
		}
		clk.advance(24 * time.Hour)
	}

	rows, err := st.db.Query(`SELECT ladder_rank FROM players ORDER BY ladder_rank`)
	if err != nil {
		t.Fatalf("query ranks: %v", err)
	}
	defer rows.Close()

	want := 1
	for rows.Next() {
		var rank int
		if err := rows.Scan(&rank); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if rank != want {
			t.Fatalf("ladder is not a permutation: expected rank %d, found %d", want, rank)
		}
		want++
	}
	if want != 42 { // 40 bots + 1 player + 1
		t.Errorf("ladder holds %d players, want 41", want-1)
	}
}
