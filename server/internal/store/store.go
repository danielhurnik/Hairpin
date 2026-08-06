// Package store owns persistence *and* the transactional game operations.
// Every rule that a player could profit from breaking — currency, timers,
// race outcomes, ladder position — is enforced inside a single database
// transaction here. Handlers do argument parsing and nothing else.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

var (
	ErrNotFound       = errors.New("not found")
	ErrInsufficient   = errors.New("insufficient funds")
	ErrInvalid        = errors.New("invalid request")
	ErrJobActive      = errors.New("a job is already running")
	ErrJobNotDone     = errors.New("job is not finished yet")
	ErrNoJob          = errors.New("no active job")
	ErrNoRaces        = errors.New("no races remaining today")
	ErrMaxTier        = errors.New("already at maximum tier")
	ErrNoOpponent     = errors.New("no opponent available")
	ErrAlreadyOwned   = errors.New("already owned")
	ErrOfferPurchased = errors.New("offer already purchased")
)

type Store struct {
	db        *sql.DB
	timeScale float64
	now       func() time.Time
}

type Options struct {
	TimeScale float64
	Now       func() time.Time
}

func Open(dsn string, opts Options) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite takes one writer at a time. Serialising the pool removes an
	// entire class of BUSY errors; this is the first thing to revisit when
	// swapping in Postgres.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	ts := opts.TimeScale
	if ts <= 0 {
		ts = 1
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Store{db: db, timeScale: ts, now: nowFn}, nil
}

func (s *Store) Close() error   { return s.db.Close() }
func (s *Store) Now() time.Time { return s.now().UTC() }
func (s *Store) DB() *sql.DB    { return s.db }

// tx runs fn inside a transaction, rolling back on any error.
func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		t.Rollback()
		return err
	}
	return t.Commit()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

type Player struct {
	ID            string            `json:"id"`
	DisplayName   string            `json:"display_name"`
	IsBot         bool              `json:"is_bot"`
	Cash          int64             `json:"cash"`
	Rubies        int64             `json:"rubies"`
	Level         int               `json:"driver_level"`
	XP            int64             `json:"driver_xp"`
	XPToNext      int64             `json:"driver_xp_to_next"`
	UnspentPoints int               `json:"unspent_points"`
	Stats         game.DriverStats  `json:"base_stats"`
	ChassisID     string            `json:"chassis_id"`
	Tiers         game.UpgradeTiers `json:"upgrades"`
	LiveryID      string            `json:"livery_id"`
	Rank          int               `json:"ladder_rank"`
	Wins          int               `json:"wins"`
	Losses        int               `json:"losses"`
	FreeRaces     int               `json:"free_races"`
	ExtraUsed     int               `json:"extra_races_used"`
	RacesResetAt  time.Time         `json:"races_reset_at"`
	CreatedAt     time.Time         `json:"created_at"`
}

type InventoryItem struct {
	ID         int64           `json:"id"`
	ItemID     string          `json:"item_id"`
	Name       string          `json:"name"`
	Slot       string          `json:"slot"`
	Rarity     game.Rarity     `json:"rarity"`
	Flavour    string          `json:"flavour"`
	Quality    float64         `json:"quality"`
	Equipped   bool            `json:"equipped"`
	Source     string          `json:"source"`
	SellValue  int64           `json:"sell_value"`
	Bonus      game.EquipBonus `json:"-"`
	AcquiredAt time.Time       `json:"acquired_at"`
}

const playerCols = `id, display_name, is_bot, cash, rubies, driver_level, driver_xp,
	unspent_points, stat_bravery, stat_intelligence, stat_reflexes, stat_consistency,
	stat_stamina, chassis_id, tier_engine, tier_turbo, tier_tires, tier_suspension,
	tier_weight, livery_id, ladder_rank, wins, losses, free_races, extra_races_used,
	races_reset_at, created_at`

func scanPlayer(row interface{ Scan(...any) error }) (*Player, error) {
	var p Player
	var isBot int
	var resetAt, createdAt int64
	err := row.Scan(&p.ID, &p.DisplayName, &isBot, &p.Cash, &p.Rubies, &p.Level, &p.XP,
		&p.UnspentPoints, &p.Stats.Bravery, &p.Stats.Intelligence, &p.Stats.Reflexes,
		&p.Stats.Consistency, &p.Stats.Stamina, &p.ChassisID, &p.Tiers.Engine,
		&p.Tiers.Turbo, &p.Tiers.Tires, &p.Tiers.Suspension, &p.Tiers.Weight,
		&p.LiveryID, &p.Rank, &p.Wins, &p.Losses, &p.FreeRaces, &p.ExtraUsed,
		&resetAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.IsBot = isBot != 0
	p.RacesResetAt = time.Unix(resetAt, 0).UTC()
	p.CreatedAt = time.Unix(createdAt, 0).UTC()
	p.XPToNext = game.XPToNext(p.Level)
	return &p, nil
}

func getPlayer(ctx context.Context, q queryer, id string) (*Player, error) {
	return scanPlayer(q.QueryRowContext(ctx, `SELECT `+playerCols+` FROM players WHERE id = ?`, id))
}

func (s *Store) GetPlayer(ctx context.Context, id string) (*Player, error) {
	return getPlayer(ctx, s.db, id)
}

// ---------------------------------------------------------------------------
// Builds
// ---------------------------------------------------------------------------

// EquippedFor loads a player's equipped items.
func equippedFor(ctx context.Context, q queryer, playerID string) ([]InventoryItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, item_id, slot, quality, equipped, source, acquired_at
		FROM inventory WHERE player_id = ? AND equipped = 1`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInventory(rows)
}

func scanInventory(rows *sql.Rows) ([]InventoryItem, error) {
	var out []InventoryItem
	for rows.Next() {
		var it InventoryItem
		var eq int
		var acquired int64
		if err := rows.Scan(&it.ID, &it.ItemID, &it.Slot, &it.Quality, &eq, &it.Source, &acquired); err != nil {
			return nil, err
		}
		it.Equipped = eq != 0
		it.AcquiredAt = time.Unix(acquired, 0).UTC()
		if def, ok := game.ItemByID(it.ItemID); ok {
			it.Name = def.Name
			it.Rarity = def.Rarity
			it.Flavour = def.Flavour
			it.Bonus = game.ItemBonus(def, it.Quality)
			it.SellValue = game.SellValue(def, it.Quality)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// buildFor derives the race-ready build. Nothing here is read from storage as
// a precomputed stat — it is always recomputed from tiers plus equipment.
func buildFor(ctx context.Context, q queryer, p *Player) (game.Build, []InventoryItem, error) {
	items, err := equippedFor(ctx, q, p.ID)
	if err != nil {
		return game.Build{}, nil, err
	}
	bonuses := make([]game.EquipBonus, 0, len(items))
	for _, it := range items {
		bonuses = append(bonuses, it.Bonus)
	}
	total := game.SumBonuses(bonuses...)
	return game.Build{
		Car:    game.ComputeCar(p.ChassisID, p.Tiers, total),
		Driver: p.Stats.Add(total.Driver),
	}, items, nil
}

// BuildFor is the exported read-only form.
func (s *Store) BuildFor(ctx context.Context, p *Player) (game.Build, []InventoryItem, error) {
	return buildFor(ctx, s.db, p)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // a broken CSPRNG is not something to paper over
	}
	return hex.EncodeToString(b)
}

func randomUint64() uint64 {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}

func touch(ctx context.Context, q queryer, playerID string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE players SET updated_at = ? WHERE id = ?`, now.Unix(), playerID)
	return err
}

// applyDailyReset refills race allowance if the player's day has rolled over.
// Called at the start of every race-related transaction rather than by a cron,
// so it cannot be dodged by never opening the app.
func applyDailyReset(ctx context.Context, q queryer, p *Player, now time.Time) error {
	if now.Before(p.RacesResetAt) {
		return nil
	}
	reset := game.NextDailyReset(now)
	p.FreeRaces = game.FreeRacesPerDay
	p.ExtraUsed = 0
	p.RacesResetAt = reset
	_, err := q.ExecContext(ctx, `UPDATE players SET free_races = ?, extra_races_used = 0, races_reset_at = ? WHERE id = ?`,
		p.FreeRaces, reset.Unix(), p.ID)
	return err
}
