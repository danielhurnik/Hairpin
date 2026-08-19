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
	ErrLevelLocked    = errors.New("driver level too low")
	ErrPartMissing    = errors.New("the car has no adjustable part for that")
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
	if err := migrateCars(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate cars: %w", err)
	}
	if err := migrateSetups(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate setups: %w", err)
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

// migrateCars moves upgrade tiers off `players` and onto `player_cars`.
//
// schema.sql handles anything expressible as CREATE ... IF NOT EXISTS; this is
// for the one change that has to carry data. It is guarded on the old column
// still being present, so it runs exactly once and is a no-op on a database
// created fresh from the current schema. The whole thing is one transaction:
// either every player keeps their build on a car row, or nothing moves.
func migrateCars(db *sql.DB) error {
	old, err := hasColumn(db, "players", "tier_engine")
	if err != nil || !old {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Every existing player had exactly one implicit car — the one named by
	// players.chassis_id. Give them that car, with the build they already paid
	// for, so nobody logs in to a stock engine.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO player_cars
		(player_id, chassis_id, tier_engine, tier_turbo, tier_tires, tier_suspension, tier_weight, acquired_at)
		SELECT id, chassis_id, tier_engine, tier_turbo, tier_tires, tier_suspension, tier_weight, created_at
		FROM players`); err != nil {
		return err
	}
	for _, col := range []string{"tier_engine", "tier_turbo", "tier_tires", "tier_suspension", "tier_weight"} {
		if _, err := tx.Exec(`ALTER TABLE players DROP COLUMN ` + col); err != nil {
			return fmt.Errorf("drop %s: %w", col, err)
		}
	}
	return tx.Commit()
}

// migrateSetups adds the setup sheet to cars that predate it. Every column
// defaults to zero, which is the factory setting, so existing cars come
// through driving exactly as they did before.
//
// The columns hold a position on an axis rather than a notch, so they are
// REAL. A database created before the sheet became continuous declared them
// INTEGER, which needs no rebuild: SQLite's integer affinity converts a value
// only when it can do so losslessly, so -0.6 is stored and read back as -0.6
// either way. The declared type is tidiness, not correctness.
func migrateSetups(db *sql.DB) error {
	has, err := hasColumn(db, "player_cars", "setup_gearing")
	if err != nil || has {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, col := range setupColumns {
		if _, err := tx.Exec(`ALTER TABLE player_cars ADD COLUMN ` + col + ` REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add %s: %w", col, err)
		}
	}
	return tx.Commit()
}

// setupColumns is the storage order of the setup sheet, used by the migration
// and by every read and write of a car.
var setupColumns = []string{
	"setup_gearing", "setup_boost", "setup_pressure", "setup_ballast",
	"setup_ride", "setup_camber", "setup_damping",
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
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
	// Setup is the active car's sheet, as stored. Always read through
	// game.LegalSetup before it reaches the race — a tier that no longer
	// covers an axis must not leave a live adjustment behind it.
	Setup        game.Setup `json:"setup"`
	LiveryID     string     `json:"livery_id"`
	Rank         int        `json:"ladder_rank"`
	Wins         int        `json:"wins"`
	Losses       int        `json:"losses"`
	FreeRaces    int        `json:"free_races"`
	ExtraUsed    int        `json:"extra_races_used"`
	RacesResetAt time.Time  `json:"races_reset_at"`
	CreatedAt    time.Time  `json:"created_at"`
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

// playerCols reads a player together with the car they are currently in, so
// Player.Tiers always means "the active car's tiers" and every consumer —
// race, ladder, garage — keeps working without knowing cars moved tables.
//
// Always paired with playerFrom, which supplies the `p` and `c` aliases.
// COALESCE means a missing car row reads as a stock car rather than dropping
// the player out of the ladder: the join can degrade, but it cannot delete.
const playerCols = `p.id, p.display_name, p.is_bot, p.cash, p.rubies, p.driver_level, p.driver_xp,
	p.unspent_points, p.stat_bravery, p.stat_intelligence, p.stat_reflexes, p.stat_consistency,
	p.stat_stamina, p.chassis_id, COALESCE(c.tier_engine, 0), COALESCE(c.tier_turbo, 0),
	COALESCE(c.tier_tires, 0), COALESCE(c.tier_suspension, 0), COALESCE(c.tier_weight, 0),
	p.livery_id, p.ladder_rank, p.wins, p.losses, p.free_races, p.extra_races_used,
	p.races_reset_at, p.created_at,
	COALESCE(c.setup_gearing, 0), COALESCE(c.setup_boost, 0), COALESCE(c.setup_pressure, 0),
	COALESCE(c.setup_ballast, 0), COALESCE(c.setup_ride, 0), COALESCE(c.setup_camber, 0),
	COALESCE(c.setup_damping, 0)`

const playerFrom = ` FROM players p
	LEFT JOIN player_cars c ON c.player_id = p.id AND c.chassis_id = p.chassis_id `

func scanPlayer(row interface{ Scan(...any) error }) (*Player, error) {
	var p Player
	var isBot int
	var resetAt, createdAt int64
	err := row.Scan(&p.ID, &p.DisplayName, &isBot, &p.Cash, &p.Rubies, &p.Level, &p.XP,
		&p.UnspentPoints, &p.Stats.Bravery, &p.Stats.Intelligence, &p.Stats.Reflexes,
		&p.Stats.Consistency, &p.Stats.Stamina, &p.ChassisID, &p.Tiers.Engine,
		&p.Tiers.Turbo, &p.Tiers.Tires, &p.Tiers.Suspension, &p.Tiers.Weight,
		&p.LiveryID, &p.Rank, &p.Wins, &p.Losses, &p.FreeRaces, &p.ExtraUsed,
		&resetAt, &createdAt,
		&p.Setup.Gearing, &p.Setup.Boost, &p.Setup.Pressure, &p.Setup.Ballast,
		&p.Setup.Ride, &p.Setup.Camber, &p.Setup.Damping)
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
	return scanPlayer(q.QueryRowContext(ctx, `SELECT `+playerCols+playerFrom+` WHERE p.id = ?`, id))
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
	// Non-nil so an empty inventory marshals as [] rather than null. A new
	// player has nothing equipped, and the client maps over these directly.
	out := []InventoryItem{}
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
	return game.WithSetup(game.Build{
		Car:    game.ComputeCar(p.ChassisID, p.Tiers, total),
		Driver: p.Stats.Add(total.Driver),
	}, game.LegalSetup(p.Setup, p.Tiers)), items, nil
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

// execGuarded runs a conditional UPDATE — the `AND cash >= ?` style guards on
// every currency path — and reports whether it actually changed a row.
//
// Checking the result is what makes those guards load-bearing. Today the pool
// is capped at one connection, so transactions serialise and the guard can
// never lose; under Postgres with concurrent transactions it can, and an
// unchecked no-op would report a successful purchase that never happened.
// `notEnough` is returned in that case.
func execGuarded(ctx context.Context, q queryer, notEnough error, query string, args ...any) error {
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notEnough
	}
	return nil
}
