package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/config"
	"github.com/danielhurnik/hairpin/server/internal/game"
	"github.com/danielhurnik/hairpin/server/internal/store"
)

// clock lets tests jump forward instead of sleeping through job timers.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type harness struct {
	t     *testing.T
	srv   *httptest.Server
	token string
	clock *clock
	store *store.Store
}

func newHarness(t *testing.T, bots int) *harness {
	t.Helper()
	clk := &clock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"

	st, err := store.Open(dsn, store.Options{TimeScale: 1, Now: clk.now})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	if _, err := st.SeedBots(context.Background(), bots); err != nil {
		t.Fatalf("seed bots: %v", err)
	}

	cfg := config.Config{
		TokenSecret: []byte("test-secret-not-for-production"),
		TokenTTL:    time.Hour,
		TimeScale:   1,
		DevIAP:      true,
		// The limiter is exercised by TestRateLimiterRejectsFloods; the loop
		// tests fire faster than any real client and would trip it.
		RatePerSec: 1e6,
		RateBurst:  1e6,
	}
	srv := httptest.NewServer(New(cfg, st).Handler())
	t.Cleanup(srv.Close)

	h := &harness{t: t, srv: srv, clock: clk, store: st}
	h.login("device-secret-abcdefghijklmnop")
	return h
}

func (h *harness) login(deviceID string) {
	h.t.Helper()
	var out struct {
		Token  string `json:"token"`
		Player struct {
			ID   string `json:"id"`
			Cash int64  `json:"cash"`
		} `json:"player"`
	}
	h.do("POST", "/v1/auth/device", map[string]any{"device_id": deviceID}, &out)
	if out.Token == "" {
		h.t.Fatal("no token returned from device auth")
	}
	h.token = out.Token
}

// do performs a request and fails the test on any non-2xx response.
func (h *harness) do(method, path string, body any, out any) {
	h.t.Helper()
	code := h.try(method, path, body, out)
	if code < 200 || code >= 300 {
		h.t.Fatalf("%s %s: unexpected status %d", method, path, code)
	}
}

// try performs a request and returns the status code without failing.
func (h *harness) try(method, path string, body any, out any) int {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, h.srv.URL+path, &buf)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			h.t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

// TestFullLoop walks the entire milestone-1 loop: sign in, work a job, claim
// the cash, spend it on the car, race a rival, and move on the ladder.
func TestFullLoop(t *testing.T) {
	h := newHarness(t, 60)

	// --- 1. initial state ---
	var state struct {
		Player struct {
			ID     string `json:"id"`
			Cash   int64  `json:"cash"`
			Rubies int64  `json:"rubies"`
			Rank   int    `json:"ladder_rank"`
			Free   int    `json:"free_races"`
		} `json:"player"`
		Rating float64 `json:"rating"`
		Ladder []struct {
			Rank  int  `json:"rank"`
			IsYou bool `json:"is_you"`
		} `json:"ladder"`
	}
	h.do("GET", "/v1/state", nil, &state)

	if state.Player.Cash != store.StartingCash {
		t.Errorf("starting cash = %d, want %d", state.Player.Cash, store.StartingCash)
	}
	if state.Player.Rank != 61 {
		t.Errorf("new player should start at the bottom (rank 61), got %d", state.Player.Rank)
	}
	if state.Rating <= 0 {
		t.Errorf("rating should be computed server-side, got %v", state.Rating)
	}
	if state.Player.Free != game.FreeRacesPerDay {
		t.Errorf("free races = %d, want %d", state.Player.Free, game.FreeRacesPerDay)
	}

	// --- 2. take a job ---
	var started struct {
		Active struct {
			JobID      string `json:"job_id"`
			RemainingS int64  `json:"remaining_seconds"`
			Ready      bool   `json:"ready"`
		} `json:"active"`
	}
	h.do("POST", "/v1/jobs/start", map[string]any{"job_id": "tofu_run"}, &started)
	if started.Active.Ready {
		t.Error("a job should not be instantly ready")
	}
	if started.Active.RemainingS <= 0 {
		t.Errorf("remaining_seconds = %d, want > 0", started.Active.RemainingS)
	}

	// Claiming early must be refused by the server clock, not the client.
	if code := h.try("POST", "/v1/jobs/claim", nil, nil); code != http.StatusConflict {
		t.Errorf("early claim status = %d, want 409", code)
	}
	// Starting a second job while one runs must be refused.
	if code := h.try("POST", "/v1/jobs/start", map[string]any{"job_id": "tofu_run"}, nil); code != http.StatusConflict {
		t.Errorf("double job start status = %d, want 409", code)
	}

	// --- 3. time passes, claim ---
	h.clock.advance(31 * time.Second)
	var claim struct {
		Cash      int64 `json:"cash"`
		TotalCash int64 `json:"total_cash"`
	}
	h.do("POST", "/v1/jobs/claim", nil, &claim)
	if claim.Cash != 45 {
		t.Errorf("claim paid %d, want 45", claim.Cash)
	}
	if claim.TotalCash != store.StartingCash+45 {
		t.Errorf("total cash = %d, want %d", claim.TotalCash, store.StartingCash+45)
	}
	// Double-claiming the same job must pay nothing.
	if code := h.try("POST", "/v1/jobs/claim", nil, nil); code != http.StatusConflict {
		t.Errorf("double claim status = %d, want 409", code)
	}

	// --- 4. buy an upgrade ---
	var before struct {
		Rating float64 `json:"rating"`
	}
	h.do("GET", "/v1/garage", nil, &before)

	var bought struct {
		Paid   int64   `json:"paid"`
		Rating float64 `json:"rating"`
		Player struct {
			Cash     int64 `json:"cash"`
			Upgrades struct {
				Tires int `json:"tires"`
			} `json:"upgrades"`
		} `json:"player"`
	}
	h.do("POST", "/v1/garage/upgrade", map[string]any{"category": "tires"}, &bought)
	if bought.Paid != 180 {
		t.Errorf("tires tier 1 cost %d, want 180", bought.Paid)
	}
	if bought.Player.Upgrades.Tires != 1 {
		t.Errorf("tires tier = %d, want 1", bought.Player.Upgrades.Tires)
	}
	if bought.Player.Cash != claim.TotalCash-180 {
		t.Errorf("cash after upgrade = %d, want %d", bought.Player.Cash, claim.TotalCash-180)
	}
	if bought.Rating <= before.Rating {
		t.Errorf("rating should rise after an upgrade: %.2f -> %.2f", before.Rating, bought.Rating)
	}

	// --- 5. race ---
	var opp struct {
		Opponent struct {
			Name   string  `json:"name"`
			Rank   int     `json:"ladder_rank"`
			Rating float64 `json:"rating"`
		} `json:"opponent"`
	}
	h.do("GET", "/v1/race/opponent", nil, &opp)
	if opp.Opponent.Name == "" {
		t.Fatal("no opponent offered")
	}
	if opp.Opponent.Rank >= state.Player.Rank {
		t.Errorf("opponent rank %d should be above the player's %d", opp.Opponent.Rank, state.Player.Rank)
	}

	var race struct {
		Won        bool `json:"won"`
		RankBefore int  `json:"rank_before"`
		RankAfter  int  `json:"rank_after"`
		FreeRaces  int  `json:"free_races_left"`
		Replay     struct {
			Segments []struct {
				Name      string  `json:"name"`
				ChallTime float64 `json:"challenger_time"`
				Gap       float64 `json:"gap"`
			} `json:"segments"`
			ChallTotal float64 `json:"challenger_total"`
			OppTotal   float64 `json:"opponent_total"`
			TrackName  string  `json:"track_name"`
		} `json:"replay"`
		Reward struct {
			Cash int64 `json:"cash"`
			XP   int64 `json:"xp"`
		} `json:"reward"`
	}
	h.do("POST", "/v1/race/start", nil, &race)

	if len(race.Replay.Segments) < 8 {
		t.Errorf("replay has %d segments, want a full corner-by-corner run", len(race.Replay.Segments))
	}
	if race.Replay.TrackName == "" {
		t.Error("replay is missing a track name")
	}
	if race.Replay.ChallTotal <= 0 || race.Replay.OppTotal <= 0 {
		t.Error("replay totals should be positive")
	}
	if race.Reward.Cash <= 0 || race.Reward.XP <= 0 {
		t.Errorf("a race should always pay something: %+v", race.Reward)
	}
	if race.FreeRaces != game.FreeRacesPerDay-1 {
		t.Errorf("free races left = %d, want %d", race.FreeRaces, game.FreeRacesPerDay-1)
	}
	if race.Won && race.RankAfter >= race.RankBefore {
		t.Errorf("a win against a higher rank should climb: %d -> %d", race.RankBefore, race.RankAfter)
	}
	if !race.Won && race.RankAfter != race.RankBefore {
		t.Errorf("a loss should not move the ladder: %d -> %d", race.RankBefore, race.RankAfter)
	}

	// --- 6. history and replay are persisted ---
	var hist struct {
		Races []struct {
			RaceID string `json:"race_id"`
			Won    bool   `json:"won"`
		} `json:"races"`
	}
	h.do("GET", "/v1/race/history", nil, &hist)
	if len(hist.Races) != 1 {
		t.Fatalf("history has %d races, want 1", len(hist.Races))
	}
	var replay struct {
		Segments []struct {
			Name string `json:"name"`
		} `json:"segments"`
	}
	h.do("GET", "/v1/race/replay/"+hist.Races[0].RaceID, nil, &replay)
	if len(replay.Segments) != len(race.Replay.Segments) {
		t.Errorf("stored replay has %d segments, live result had %d",
			len(replay.Segments), len(race.Replay.Segments))
	}
}

// TestDailyRaceLimit checks that free races run out, that rubies buy more, and
// that the allowance comes back the next day.
func TestDailyRaceLimit(t *testing.T) {
	h := newHarness(t, 40)

	for i := 0; i < game.FreeRacesPerDay; i++ {
		if code := h.try("POST", "/v1/race/start", nil, nil); code != http.StatusOK {
			t.Fatalf("free race %d returned %d", i+1, code)
		}
	}

	// Starting rubies (50) cover three extra races at 15 each.
	var p struct {
		Player struct {
			Rubies int64 `json:"rubies"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &p)
	if p.Player.Rubies != store.StartingRubies {
		t.Fatalf("rubies = %d, want %d", p.Player.Rubies, store.StartingRubies)
	}

	for i := 0; i < 3; i++ {
		var out struct {
			SpentRuby int64 `json:"rubies_spent"`
		}
		if code := h.try("POST", "/v1/race/start", nil, &out); code != http.StatusOK {
			t.Fatalf("paid race %d returned %d", i+1, code)
		}
		if out.SpentRuby != game.ExtraRaceRubies {
			t.Errorf("paid race spent %d rubies, want %d", out.SpentRuby, game.ExtraRaceRubies)
		}
	}

	// 50 rubies - 45 spent = 5 left, not enough for a fourth.
	if code := h.try("POST", "/v1/race/start", nil, nil); code != http.StatusConflict {
		t.Errorf("race with no allowance and no rubies returned %d, want 409", code)
	}

	// Next day, the free races are back.
	h.clock.advance(24 * time.Hour)
	if code := h.try("POST", "/v1/race/start", nil, nil); code != http.StatusOK {
		t.Errorf("race after daily reset returned %d, want 200", code)
	}
}

// TestEconomyCannotBeCheated covers the obvious client-side attacks.
func TestEconomyCannotBeCheated(t *testing.T) {
	h := newHarness(t, 20)

	// Cannot buy something you cannot afford.
	if code := h.try("POST", "/v1/garage/upgrade", map[string]any{"category": "turbo"}, nil); code != http.StatusOK {
		t.Fatalf("turbo tier 1 costs 400 and the player has 500: got %d", code)
	}
	// Second tier costs 1400; the player now has 100.
	if code := h.try("POST", "/v1/garage/upgrade", map[string]any{"category": "turbo"}, nil); code != http.StatusPaymentRequired {
		t.Errorf("unaffordable upgrade returned %d, want 402", code)
	}

	// Unknown categories are rejected outright.
	if code := h.try("POST", "/v1/garage/upgrade", map[string]any{"category": "nitrous"}, nil); code != http.StatusBadRequest {
		t.Errorf("bogus category returned %d, want 400", code)
	}

	// Extra JSON fields (a classic "just add cash: 999999" attempt) are refused.
	if code := h.try("POST", "/v1/garage/upgrade",
		map[string]any{"category": "tires", "cash": 999999}, nil); code != http.StatusBadRequest {
		t.Errorf("request with unknown fields returned %d, want 400", code)
	}

	// Allocating stat points you have not earned fails.
	if code := h.try("POST", "/v1/driver/allocate",
		map[string]any{"stat": "bravery", "points": 50}, nil); code != http.StatusPaymentRequired {
		t.Errorf("over-allocating returned %d, want 402", code)
	}

	// No token at all.
	saved := h.token
	h.token = ""
	if code := h.try("GET", "/v1/state", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request returned %d, want 401", code)
	}
	// A forged token.
	h.token = "v1.aaaa.bbbb"
	if code := h.try("GET", "/v1/state", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("forged token returned %d, want 401", code)
	}
	h.token = saved
}

// TestRushJobSpendsRubies covers the premium-currency shortcut.
func TestRushJobSpendsRubies(t *testing.T) {
	h := newHarness(t, 10)
	h.do("POST", "/v1/jobs/start", map[string]any{"job_id": "tofu_run"}, nil)

	var rush struct {
		Spent  int64 `json:"rubies_spent"`
		Active struct {
			Ready bool `json:"ready"`
		} `json:"active"`
	}
	h.do("POST", "/v1/jobs/rush", nil, &rush)
	if rush.Spent < 1 {
		t.Errorf("rush spent %d rubies, want at least 1", rush.Spent)
	}
	if !rush.Active.Ready {
		t.Error("job should be claimable immediately after a rush")
	}

	var claim struct {
		Cash int64 `json:"cash"`
	}
	h.do("POST", "/v1/jobs/claim", nil, &claim)
	if claim.Cash != 45 {
		t.Errorf("rushed job paid %d, want the normal 45", claim.Cash)
	}

	var me struct {
		Player struct {
			Rubies int64 `json:"rubies"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &me)
	if me.Player.Rubies >= store.StartingRubies {
		t.Errorf("rubies should have been spent: %d", me.Player.Rubies)
	}
}

// TestLockedJobsRequireLevel keeps the progression gates server-side.
func TestLockedJobsRequireLevel(t *testing.T) {
	h := newHarness(t, 10)
	if code := h.try("POST", "/v1/jobs/start", map[string]any{"job_id": "shop_shift"}, nil); code != http.StatusBadRequest {
		t.Errorf("starting a level-15 job at level 1 returned %d, want 400", code)
	}
}

// TestShopIsStableAndSingleUse covers K's Office.
func TestShopIsStableAndSingleUse(t *testing.T) {
	h := newHarness(t, 10)

	var shop1, shop2 struct {
		DayKey string `json:"day_key"`
		Offers []struct {
			OfferID   string `json:"offer_id"`
			ItemID    string `json:"item_id"`
			PriceCash int64  `json:"price_cash"`
			PriceRuby int64  `json:"price_rubies"`
			SoldOut   bool   `json:"sold_out"`
		} `json:"offers"`
	}
	h.do("GET", "/v1/shop", nil, &shop1)
	h.do("GET", "/v1/shop", nil, &shop2)

	if len(shop1.Offers) == 0 {
		t.Fatal("K's Office is empty")
	}
	// Refreshing must not reroll the stock.
	for i := range shop1.Offers {
		if shop1.Offers[i].OfferID != shop2.Offers[i].OfferID {
			t.Fatalf("shop rerolled between requests: %s vs %s",
				shop1.Offers[i].OfferID, shop2.Offers[i].OfferID)
		}
	}

	// Buying an offer that does not exist is refused.
	if code := h.try("POST", "/v1/shop/buy", map[string]any{"offer_id": "deadbeefdeadbeef"}, nil); code != http.StatusNotFound {
		t.Errorf("bogus offer returned %d, want 404", code)
	}

	// Find something affordable with the starting 500 cash.
	target := ""
	for _, o := range shop1.Offers {
		if o.PriceCash > 0 && o.PriceCash <= store.StartingCash {
			target = o.OfferID
			break
		}
	}
	if target == "" {
		t.Skip("no affordable offer in today's roll for this seed")
	}
	var buy struct {
		Item struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"item"`
	}
	h.do("POST", "/v1/shop/buy", map[string]any{"offer_id": target}, &buy)
	if buy.Item.ID == 0 {
		t.Fatal("purchase did not return an item")
	}
	// The same offer cannot be bought twice.
	if code := h.try("POST", "/v1/shop/buy", map[string]any{"offer_id": target}, nil); code != http.StatusConflict {
		t.Errorf("repeat purchase returned %d, want 409", code)
	}

	// Equipping the item must change the build the server computes.
	var beforeEquip struct {
		Rating float64 `json:"rating"`
	}
	h.do("GET", "/v1/me", nil, &beforeEquip)

	var afterEquip struct {
		Rating float64 `json:"rating"`
	}
	h.do("POST", "/v1/inventory/equip", map[string]any{"item_id": buy.Item.ID}, &afterEquip)
	if afterEquip.Rating <= beforeEquip.Rating {
		t.Errorf("equipping %s did not improve the rating: %.3f -> %.3f",
			buy.Item.Name, beforeEquip.Rating, afterEquip.Rating)
	}
}

// TestRubyPurchaseIsIdempotent — a replayed receipt must not pay out twice.
func TestRubyPurchaseIsIdempotent(t *testing.T) {
	h := newHarness(t, 10)
	body := map[string]any{"pack_id": "pack_pouch", "receipt": "receipt-token-0001"}

	var first struct {
		Granted int64 `json:"rubies_granted"`
		Player  struct {
			Rubies int64 `json:"rubies"`
		} `json:"player"`
	}
	h.do("POST", "/v1/store/purchase", body, &first)
	if first.Granted != 600 {
		t.Errorf("granted %d rubies, want 600 (550 + 50 bonus)", first.Granted)
	}
	if first.Player.Rubies != store.StartingRubies+600 {
		t.Errorf("rubies = %d, want %d", first.Player.Rubies, store.StartingRubies+600)
	}

	if code := h.try("POST", "/v1/store/purchase", body, nil); code != http.StatusConflict {
		t.Errorf("replayed receipt returned %d, want 409", code)
	}

	var me struct {
		Player struct {
			Rubies int64 `json:"rubies"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &me)
	if me.Player.Rubies != store.StartingRubies+600 {
		t.Errorf("replay credited a second time: %d", me.Player.Rubies)
	}
}

// TestLiveriesAreCosmetic proves the headline promise about paid cosmetics.
func TestLiveriesAreCosmetic(t *testing.T) {
	h := newHarness(t, 10)

	var before struct {
		Rating float64    `json:"rating"`
		Build  game.Build `json:"build"`
	}
	h.do("GET", "/v1/me", nil, &before)

	h.do("POST", "/v1/store/purchase", map[string]any{"pack_id": "pack_case", "receipt": "livery-test-receipt"}, nil)
	h.do("POST", "/v1/liveries/buy", map[string]any{"livery_id": "impact"}, nil)
	h.do("POST", "/v1/liveries/equip", map[string]any{"livery_id": "impact"}, nil)

	var after struct {
		Rating float64    `json:"rating"`
		Build  game.Build `json:"build"`
	}
	h.do("GET", "/v1/me", nil, &after)

	if after.Rating != before.Rating {
		t.Errorf("livery changed the rating: %.4f -> %.4f", before.Rating, after.Rating)
	}
	if after.Build != before.Build {
		t.Errorf("livery changed the build:\n%+v\n%+v", before.Build, after.Build)
	}
}

// TestLadderClimbIsConsistent races many times and checks the standings stay
// a valid permutation — no duplicate or vanished ranks.
func TestLadderClimbIsConsistent(t *testing.T) {
	h := newHarness(t, 50)

	for day := 0; day < 12; day++ {
		for i := 0; i < game.FreeRacesPerDay; i++ {
			h.try("POST", "/v1/race/start", nil, nil)
		}
		h.clock.advance(24 * time.Hour)
	}

	rows, err := h.store.DB().Query(`SELECT ladder_rank, COUNT(*) FROM players GROUP BY ladder_rank ORDER BY ladder_rank`)
	if err != nil {
		t.Fatalf("query ranks: %v", err)
	}
	defer rows.Close()

	expected := 1
	for rows.Next() {
		var rank, n int
		if err := rows.Scan(&rank, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if n != 1 {
			t.Errorf("rank %d is held by %d players", rank, n)
		}
		if rank != expected {
			t.Fatalf("ladder has a hole: expected rank %d, found %d", expected, rank)
		}
		expected++
	}
	if expected != 52 { // 50 bots + 1 player + 1
		t.Errorf("ladder has %d entries, want 51", expected-1)
	}

	var me struct {
		Player struct {
			Rank   int `json:"ladder_rank"`
			Wins   int `json:"wins"`
			Losses int `json:"losses"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &me)
	if me.Player.Wins+me.Player.Losses != 60 {
		t.Errorf("recorded %d races, want 60", me.Player.Wins+me.Player.Losses)
	}
	if me.Player.Rank >= 51 {
		t.Errorf("after %d wins the player should have climbed off the bottom, rank = %d",
			me.Player.Wins, me.Player.Rank)
	}
	t.Logf("after 60 races: rank %d, %d wins / %d losses",
		me.Player.Rank, me.Player.Wins, me.Player.Losses)
}

// TestTwoPlayersRaceEachOthersBuilds is the async-multiplayer promise: the
// "bot" a player faces is another real account's saved build.
func TestTwoPlayersRaceEachOthersBuilds(t *testing.T) {
	h := newHarness(t, 3)
	firstID := ""
	var me struct {
		Player struct {
			ID string `json:"id"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &me)
	firstID = me.Player.ID

	// A second device signs in and becomes a separate account.
	h.login("second-device-secret-qrstuvwx")
	var me2 struct {
		Player struct {
			ID string `json:"id"`
		} `json:"player"`
	}
	h.do("GET", "/v1/me", nil, &me2)
	if me2.Player.ID == firstID {
		t.Fatal("two devices produced the same account")
	}

	// Race until the first player turns up as the opponent.
	found := false
	for i := 0; i < game.FreeRacesPerDay && !found; i++ {
		var race struct {
			Opponent struct {
				PlayerID string `json:"player_id"`
				IsBot    bool   `json:"is_bot"`
			} `json:"opponent"`
		}
		if code := h.try("POST", "/v1/race/start", nil, &race); code != http.StatusOK {
			break
		}
		if race.Opponent.PlayerID == firstID {
			found = true
			if race.Opponent.IsBot {
				t.Error("a real player's build was flagged as a bot")
			}
		}
	}
	if !found {
		t.Skip("did not draw the other human in the free races available")
	}
}

func TestContentEndpointNeedsNoAuth(t *testing.T) {
	h := newHarness(t, 1)
	saved := h.token
	h.token = ""
	var content struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
		MaxTier int `json:"max_tier"`
	}
	h.do("GET", "/v1/content", nil, &content)
	h.token = saved

	if content.MaxTier != game.MaxTier {
		t.Errorf("max_tier = %d, want %d", content.MaxTier, game.MaxTier)
	}
	want := map[string]bool{"keiichi_gloves": false, "takumi_brakes": false, "ryosuke_knowledge": false}
	for _, it := range content.Items {
		if _, ok := want[it.ID]; ok {
			want[it.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("content is missing the named item %q", id)
		}
	}
}

func TestDropsEventuallyIncludeLegendaries(t *testing.T) {
	// Not an API test — a statistical check that the rarest items can actually
	// reach a player at the top of the ladder.
	seen := map[string]int{}
	rng := newTestRNG()
	for i := 0; i < 200000; i++ {
		if d, ok := game.RollDrop(rng, 1.0, 1.0); ok {
			if def, _ := game.ItemByID(d.ItemID); def.Rarity == game.Legendary {
				seen[d.ItemID]++
			}
		}
	}
	for _, id := range []string{"keiichi_gloves", "takumi_brakes", "ryosuke_knowledge"} {
		if seen[id] == 0 {
			t.Errorf("%s never dropped in 200k top-tier rolls", id)
		}
	}
	total := 0
	for _, n := range seen {
		total += n
	}
	rate := float64(total) / 200000
	if rate < 0.01 || rate > 0.10 {
		t.Errorf("legendary rate at max quality = %.4f, want 0.01..0.10", rate)
	}
	fmt.Printf("  legendary drop rate at max quality: %.2f%%\n", rate*100)
}

// A brand-new account has an empty inventory and nothing equipped. Those
// fields must marshal as [] and not null: the client maps over them directly,
// and a null crashed the driver screen for every first-time player.
func TestEmptyCollectionsAreArraysNotNull(t *testing.T) {
	h := newHarness(t, 5)

	var raw map[string]json.RawMessage
	h.do("GET", "/v1/state", nil, &raw)

	for _, field := range []string{"equipped", "inventory", "job_board", "garage", "ladder", "liveries"} {
		v, ok := raw[field]
		if !ok {
			t.Errorf("state is missing %q", field)
			continue
		}
		if string(v) == "null" {
			t.Errorf("%q serialised as null; the client maps over it", field)
		}
	}

	var hist map[string]json.RawMessage
	h.do("GET", "/v1/race/history", nil, &hist)
	if string(hist["races"]) == "null" {
		t.Error("race history serialised as null before any race")
	}

	var inv map[string]json.RawMessage
	h.do("GET", "/v1/inventory", nil, &inv)
	if string(inv["items"]) == "null" {
		t.Error("inventory items serialised as null when empty")
	}
}

// ---------------------------------------------------------------------------
// Cars
// ---------------------------------------------------------------------------

// carsView is the shape the showroom returns.
type carsView struct {
	Player struct {
		ID        string `json:"id"`
		Cash      int64  `json:"cash"`
		ChassisID string `json:"chassis_id"`
	} `json:"player"`
	Active string  `json:"active"`
	Rating float64 `json:"rating"`
	Cars   []struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Price    int64   `json:"price"`
		MinLevel int     `json:"min_level"`
		Owned    bool    `json:"owned"`
		Active   bool    `json:"active"`
		Buyable  bool    `json:"buyable"`
		Rating   float64 `json:"rating"`
		Upgrades struct {
			Engine int `json:"engine"`
		} `json:"upgrades"`
	} `json:"cars"`
}

func (v carsView) car(t *testing.T, id string) int {
	t.Helper()
	for i, c := range v.Cars {
		if c.ID == id {
			return i
		}
	}
	t.Fatalf("chassis %s missing from the showroom", id)
	return -1
}

// grant tops a player up so a test can reach the showroom without grinding.
func (h *harness) grant(cash int64, level int) {
	h.t.Helper()
	var id string
	if err := h.store.DB().QueryRow(`SELECT id FROM players WHERE device_hash IS NOT NULL LIMIT 1`).Scan(&id); err != nil {
		h.t.Fatalf("find player: %v", err)
	}
	if _, err := h.store.DB().Exec(`UPDATE players SET cash = ?, driver_level = ? WHERE id = ?`,
		cash, level, id); err != nil {
		h.t.Fatalf("grant: %v", err)
	}
}

// TestBuyAndDriveASecondCar walks the whole feature over HTTP: look at the
// showroom, buy a car, switch to it, and race it.
func TestBuyAndDriveASecondCar(t *testing.T) {
	h := newHarness(t, 20)

	var before carsView
	h.do("GET", "/v1/cars", nil, &before)
	starter := before.Cars[before.car(t, game.StarterChassis)]
	if !starter.Owned || !starter.Active {
		t.Fatalf("starter should be owned and active, got owned=%v active=%v", starter.Owned, starter.Active)
	}
	if before.Cars[before.car(t, "eg_hatch")].Owned {
		t.Fatal("player starts owning a car they never bought")
	}

	target, _ := game.ChassisByID("eg_hatch")
	h.grant(target.Price+1000, target.MinLevel)

	var bought carsView
	h.do("POST", "/v1/cars/buy", map[string]any{"chassis_id": "eg_hatch"}, &bought)
	if bought.Player.Cash != 1000 {
		t.Errorf("cash %d after buying, want 1000", bought.Player.Cash)
	}
	if !bought.Cars[bought.car(t, "eg_hatch")].Owned {
		t.Error("bought car is not owned")
	}
	// Buying does not put you in it — that is a separate choice.
	if bought.Active != game.StarterChassis {
		t.Errorf("active car became %s on purchase", bought.Active)
	}

	var driving carsView
	h.do("POST", "/v1/cars/equip", map[string]any{"chassis_id": "eg_hatch"}, &driving)
	if driving.Active != "eg_hatch" || driving.Player.ChassisID != "eg_hatch" {
		t.Fatalf("active car is %s, want eg_hatch", driving.Active)
	}

	// The new car races. Its build is its own, so the garage now prices
	// upgrades from stock.
	var garage struct {
		Upgrades []struct {
			Category string `json:"category"`
			Tier     int    `json:"tier"`
		} `json:"upgrades"`
	}
	h.do("GET", "/v1/garage", nil, &garage)
	for _, u := range garage.Upgrades {
		if u.Tier != 0 {
			t.Errorf("%s reads tier %d on a freshly bought car, want 0", u.Category, u.Tier)
		}
	}

	var raced struct {
		Replay struct {
			TrackName string `json:"track_name"`
			Segments  []struct {
				Name string `json:"name"`
			} `json:"segments"`
		} `json:"replay"`
	}
	h.do("POST", "/v1/race/start", nil, &raced)
	if raced.Replay.TrackName == "" || len(raced.Replay.Segments) < 8 {
		t.Error("racing the newly bought car produced no full replay")
	}
}

// The showroom is a currency path, so it gets the same scrutiny as the rest.
func TestCarPurchaseCannotBeCheated(t *testing.T) {
	h := newHarness(t, 20)

	// Old enough to be sold the car, nowhere near able to afford it. The level
	// gate is checked first, so this needs the level granted to reach the
	// cash check at all.
	target, _ := game.ChassisByID("eg_hatch")
	h.grant(target.Price-1, target.MinLevel)
	if code := h.try("POST", "/v1/cars/buy", map[string]any{"chassis_id": "eg_hatch"}, nil); code != http.StatusPaymentRequired {
		t.Errorf("unaffordable car returned %d, want 402", code)
	}

	// Rich enough for the best car in the game, nowhere near the level.
	h.grant(10_000_000, 1)
	if code := h.try("POST", "/v1/cars/buy", map[string]any{"chassis_id": "r_gt"}, nil); code != http.StatusConflict {
		t.Errorf("under-level purchase returned %d, want 409", code)
	}

	// A price named by the client is not a price.
	if code := h.try("POST", "/v1/cars/buy",
		map[string]any{"chassis_id": "r_gt", "price": 1}, nil); code != http.StatusBadRequest {
		t.Errorf("request with unknown fields returned %d, want 400", code)
	}

	// Chassis that do not exist, and the starter, are both refused.
	if code := h.try("POST", "/v1/cars/buy", map[string]any{"chassis_id": "f1_car"}, nil); code != http.StatusBadRequest {
		t.Errorf("bogus chassis returned %d, want 400", code)
	}
	if code := h.try("POST", "/v1/cars/buy", map[string]any{"chassis_id": game.StarterChassis}, nil); code != http.StatusBadRequest {
		t.Errorf("re-buying the starter returned %d, want 400", code)
	}

	// You cannot drive what you do not own.
	if code := h.try("POST", "/v1/cars/equip", map[string]any{"chassis_id": "gc_awd"}, nil); code != http.StatusNotFound {
		t.Errorf("driving an unowned car returned %d, want 404", code)
	}

	// Buying the same car twice takes the money once.
	h.grant(10_000_000, 50)
	h.do("POST", "/v1/cars/buy", map[string]any{"chassis_id": "gc_awd"}, nil)
	var after carsView
	h.do("GET", "/v1/cars", nil, &after)
	spent := int64(10_000_000) - after.Player.Cash
	if code := h.try("POST", "/v1/cars/buy", map[string]any{"chassis_id": "gc_awd"}, nil); code != http.StatusConflict {
		t.Errorf("second purchase returned %d, want 409", code)
	}
	var final carsView
	h.do("GET", "/v1/cars", nil, &final)
	if got := int64(10_000_000) - final.Player.Cash; got != spent {
		t.Errorf("spent %d across two purchase attempts, want %d", got, spent)
	}
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

type setupView struct {
	Setup struct {
		ChassisID   string `json:"chassis_id"`
		AnyUnlocked bool   `json:"any_unlocked"`
		Track       struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"track"`
		Axes []struct {
			ID       string  `json:"id"`
			Value    int     `json:"value"`
			Setting  float64 `json:"setting"`
			Display  string  `json:"display"`
			Unit     string  `json:"unit"`
			Unlocked bool    `json:"unlocked"`
			Requires string  `json:"requires"`
		} `json:"axes"`
	} `json:"setup"`
	Rating float64 `json:"rating"`
}

func (v setupView) axis(t *testing.T, id string) int {
	t.Helper()
	for i, a := range v.Setup.Axes {
		if a.ID == id {
			return i
		}
	}
	t.Fatalf("axis %s missing from the sheet", id)
	return -1
}

// buildEverything maxes every category so the whole sheet is adjustable.
func (h *harness) buildEverything() {
	h.t.Helper()
	h.grant(20_000_000, 60)
	for _, cat := range game.UpgradeCategories {
		for i := 0; i < game.MaxTier; i++ {
			h.do("POST", "/v1/garage/upgrade", map[string]any{"category": cat}, nil)
		}
	}
}

// The load-bearing promise of the feature: you are shown a road, you set the
// car up for it, and that is the road you actually race.
func TestYouRaceTheTrackYouWereShown(t *testing.T) {
	h := newHarness(t, 20)

	for i := 0; i < 4; i++ {
		var preview struct {
			Track struct {
				ID string `json:"id"`
			} `json:"track"`
		}
		h.do("GET", "/v1/race/opponent", nil, &preview)
		if preview.Track.ID == "" {
			t.Fatal("the opponent screen does not say where the race is")
		}

		// Asking again must not re-roll it.
		for j := 0; j < 3; j++ {
			var again struct {
				Track struct {
					ID string `json:"id"`
				} `json:"track"`
			}
			h.do("GET", "/v1/race/opponent", nil, &again)
			if again.Track.ID != preview.Track.ID {
				t.Fatalf("track changed from %s to %s just by looking again",
					preview.Track.ID, again.Track.ID)
			}
		}

		var raced struct {
			Replay struct {
				TrackID string `json:"track_id"`
			} `json:"replay"`
		}
		h.do("POST", "/v1/race/start", nil, &raced)
		if raced.Replay.TrackID != preview.Track.ID {
			t.Fatalf("raced on %s after being shown %s", raced.Replay.TrackID, preview.Track.ID)
		}
	}
}

func TestSetupSheetUnlocksWithParts(t *testing.T) {
	h := newHarness(t, 20)

	var stock setupView
	h.do("GET", "/v1/setup", nil, &stock)
	if stock.Setup.AnyUnlocked {
		t.Error("a stock car reports something adjustable")
	}
	if len(stock.Setup.Axes) == 0 {
		t.Fatal("the sheet is empty")
	}
	for _, a := range stock.Setup.Axes {
		if a.Requires == "" {
			t.Errorf("locked axis %q does not say which part to buy", a.ID)
		}
	}

	h.buildEverything()
	var built setupView
	h.do("GET", "/v1/setup", nil, &built)
	if !built.Setup.AnyUnlocked {
		t.Fatal("a fully built car still reports nothing adjustable")
	}
	for _, a := range built.Setup.Axes {
		if !a.Unlocked {
			t.Errorf("axis %q locked on a fully built car", a.ID)
		}
	}

	// Tuning sticks, reports itself in real units, and leaves the garage
	// number alone.
	before := built
	var after setupView
	h.do("POST", "/v1/setup", map[string]any{"axis": "gearing", "value": -2}, &after)
	row := after.Setup.Axes[after.axis(t, "gearing")]
	if row.Value != -2 {
		t.Error("gearing did not stick")
	}
	if row.Display != "3.70" {
		t.Errorf("final drive at notch -2 displays as %q, want the real ratio 3.70", row.Display)
	}
	if diff := after.Rating - before.Rating; diff > 0.05 || diff < -0.05 {
		t.Errorf("overall rating moved by %.4f on a setup change", diff)
	}
}

// The sheet must not hand back a score for the road ahead. A per-track number
// responds to every adjustment, so publishing it would let the first player to
// optimise against it solve the pass for everybody.
func TestSetupDoesNotPredictWhatASheetIsWorth(t *testing.T) {
	h := newHarness(t, 20)
	h.buildEverything()

	for _, path := range []string{"/v1/setup", "/v1/race/opponent", "/v1/state"} {
		var raw map[string]any
		h.do("GET", path, nil, &raw)
		body, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal %s: %v", path, err)
		}
		for _, leak := range []string{"track_rating", "\"effect\"", "\"matters\""} {
			if bytes.Contains(body, []byte(leak)) {
				t.Errorf("%s exposes %s — that is a solver, not a setup sheet", path, leak)
			}
		}
	}
}

func TestSetupCannotBeCheated(t *testing.T) {
	h := newHarness(t, 20)

	// Nothing is adjustable until the part is on the car.
	if code := h.try("POST", "/v1/setup", map[string]any{"axis": "camber", "value": 2}, nil); code != http.StatusConflict {
		t.Errorf("adjusting a part-less car returned %d, want 409", code)
	}

	h.buildEverything()

	// Values off the end of the sheet.
	for _, v := range []int{game.SetupMax + 1, game.SetupMin - 1, 99999} {
		if code := h.try("POST", "/v1/setup", map[string]any{"axis": "gearing", "value": v}, nil); code != http.StatusBadRequest {
			t.Errorf("gearing %d returned %d, want 400", v, code)
		}
	}
	// An axis that does not exist.
	if code := h.try("POST", "/v1/setup", map[string]any{"axis": "nitrous", "value": 1}, nil); code != http.StatusBadRequest {
		t.Errorf("bogus axis returned %d, want 400", code)
	}
	// Naming the effect rather than the setting.
	if code := h.try("POST", "/v1/setup",
		map[string]any{"axis": "gearing", "value": 1, "rating": 999}, nil); code != http.StatusBadRequest {
		t.Errorf("request with unknown fields returned %d, want 400", code)
	}

	// A rival's sheet is not readable — copying is not tuning.
	var op map[string]any
	h.do("GET", "/v1/race/opponent", nil, &op)
	body, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(body, []byte("\"setup\"")) {
		t.Error("the opponent view leaks the rival's setup sheet")
	}
}
