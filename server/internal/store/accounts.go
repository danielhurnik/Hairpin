package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// EnsurePlayer finds or creates the account behind a device hash. New players
// land at the bottom of the ladder with a stock car and the starter livery.
func (s *Store) EnsurePlayer(ctx context.Context, deviceHash string) (*Player, bool, error) {
	var created bool
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := scanPlayer(tx.QueryRowContext(ctx,
			`SELECT `+playerCols+playerFrom+` WHERE p.device_hash = ?`, deviceHash))
		if err == nil {
			out = p
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		now := s.Now()
		id := NewID()
		var bottom int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ladder_rank), 0) FROM players`).Scan(&bottom); err != nil {
			return err
		}
		st := game.StartingDriverStats()
		_, err = tx.ExecContext(ctx, `INSERT INTO players
			(id, device_hash, display_name, is_bot, cash, rubies, driver_level, driver_xp,
			 unspent_points, stat_bravery, stat_intelligence, stat_reflexes, stat_consistency,
			 stat_stamina, chassis_id, livery_id, ladder_rank, free_races, races_reset_at,
			 created_at, updated_at)
			VALUES (?, ?, ?, 0, ?, ?, 1, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, deviceHash, newDriverName(), StartingCash, StartingRubies,
			st.Bravery, st.Intelligence, st.Reflexes, st.Consistency, st.Stamina,
			game.StarterChassis, game.StarterLivery, bottom+1,
			game.FreeRacesPerDay, game.NextDailyReset(now).Unix(), now.Unix(), now.Unix())
		if err != nil {
			return err
		}
		// The starter car is owned, not bought — it needs a row like any other
		// car so the active-car join finds a build to race.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO player_cars (player_id, chassis_id, acquired_at) VALUES (?, ?, ?)`,
			id, game.StarterChassis, now.Unix()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO player_liveries (player_id, livery_id, acquired_at) VALUES (?, ?, ?)`,
			id, game.StarterLivery, now.Unix()); err != nil {
			return err
		}
		created = true
		out, err = getPlayer(ctx, tx, id)
		return err
	})
	return out, created, err
}

const (
	StartingCash   = 500
	StartingRubies = 50
)

// SetDisplayName lets a player rename themselves once they have picked a handle.
func (s *Store) SetDisplayName(ctx context.Context, playerID, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE players SET display_name = ?, updated_at = ? WHERE id = ?`,
		name, s.Now().Unix(), playerID)
	return err
}

// ---------------------------------------------------------------------------
// Ladder bots
//
// The ladder needs bodies before it has players. Seeded bots are ordinary
// rows in `players` with is_bot = 1 — they race with the same build pipeline
// as a human, so once real players arrive nothing about the matchmaking
// changes. A real player's build IS the bot their rivals race against.
// ---------------------------------------------------------------------------

var handlePrefixes = []string{
	"Night", "Ghost", "Blind", "Drift", "Gutter", "Shadow", "Silent", "Iron", "White",
	"Red", "Blue", "Storm", "Mountain", "Valley", "Summit", "Midnight", "Frost", "Ash",
	"Steel", "Thunder", "Quiet", "Lone", "Crimson", "Jade", "Neon", "Rust",
}

var handleSuffixes = []string{
	"Comet", "Fox", "Hawk", "Lizard", "Panther", "Runner", "Dancer", "Sniper", "Rider",
	"Blade", "Wolf", "Serpent", "Falcon", "Tiger", "Crow", "Moth", "Viper", "Bull",
	"Deer", "Heron", "Koi", "Oni", "Tanuki", "Kite", "Hornet",
}

var teamNames = []string{
	"Speed Stars", "Night Kids", "Emperor", "Red Suns", "Seven Star Leaf", "Impact Blue",
	"Purple Shadow", "Lan Evo Squad", "Todo School", "Sidewinder", "Katagiri Racing",
	"Northern Alliance",
}

func newDriverName() string {
	return fmt.Sprintf("%s%s", handlePrefixes[rand.IntN(len(handlePrefixes))], handleSuffixes[rand.IntN(len(handleSuffixes))])
}

func botName(rng *rand.Rand) string {
	n := handlePrefixes[rng.IntN(len(handlePrefixes))] + handleSuffixes[rng.IntN(len(handleSuffixes))]
	if rng.Float64() < 0.45 {
		n = teamNames[rng.IntN(len(teamNames))] + " " + n
	}
	return n
}

// SeedBots fills the ladder up to `count` bots, strongest at rank 1. It is
// idempotent: it only tops up what is missing.
func (s *Store) SeedBots(ctx context.Context, count int) (int, error) {
	if count <= 0 {
		return 0, nil
	}
	var existing int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM players WHERE is_bot = 1`).Scan(&existing); err != nil {
		return 0, err
	}
	if existing >= count {
		return 0, nil
	}

	now := s.Now()
	rng := rand.New(rand.NewPCG(0x746F756765, uint64(count)))
	added := 0

	err := s.tx(ctx, func(tx *sql.Tx) error {
		for i := existing; i < count; i++ {
			// rank 1 is the top of the mountain.
			rank := i + 1
			// strength runs 1.0 at rank 1 down to ~0.05 at the bottom.
			strength := 1 - float64(i)/float64(count)
			strength = clamp01(strength*strength*0.55 + strength*0.45)

			id := NewID()
			tiers := botTiers(rng, strength)
			level := 1 + int(strength*44) + rng.IntN(3)
			stats := botStats(rng, level, strength)
			chassis := botChassis(rng, strength)
			setup := botSetup(rng, strength, tiers)

			if _, err := tx.ExecContext(ctx, `INSERT INTO players
				(id, device_hash, display_name, is_bot, cash, rubies, driver_level, driver_xp,
				 unspent_points, stat_bravery, stat_intelligence, stat_reflexes, stat_consistency,
				 stat_stamina, chassis_id, livery_id, ladder_rank, wins, losses, free_races,
				 races_reset_at, created_at, updated_at)
				VALUES (?, NULL, ?, 1, 0, 0, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
				id, botName(rng), level,
				stats.Bravery, stats.Intelligence, stats.Reflexes, stats.Consistency, stats.Stamina,
				chassis, game.Liveries[rng.IntN(len(game.Liveries))].ID, rank,
				int(strength*120)+rng.IntN(20), int((1-strength)*60)+rng.IntN(20),
				game.NextDailyReset(now).Unix(), now.Unix(), now.Unix()); err != nil {
				return err
			}

			// A bot owns exactly the car it drives. They go through the same
			// active-car join as a human, so the ladder needs no special case.
			if _, err := tx.ExecContext(ctx, `INSERT INTO player_cars
				(player_id, chassis_id, tier_engine, tier_turbo, tier_tires, tier_suspension, tier_weight,
				 setup_gearing, setup_boost, setup_pressure, setup_ballast, setup_ride, setup_camber,
				 setup_damping, acquired_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, chassis, tiers.Engine, tiers.Turbo, tiers.Tires, tiers.Suspension, tiers.Weight,
				setup.Gearing, setup.Boost, setup.Pressure, setup.Ballast, setup.Ride, setup.Camber,
				setup.Damping, now.Unix()); err != nil {
				return err
			}

			// Strong bots carry equipment, so climbing the ladder means facing
			// the named items rather than just bigger turbos.
			for _, it := range botItems(rng, strength) {
				def, ok := game.ItemByID(it.itemID)
				if !ok {
					continue
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO inventory
					(player_id, item_id, slot, quality, equipped, source, acquired_at)
					VALUES (?, ?, ?, ?, 1, 'seed', ?)`,
					id, def.ID, def.Slot, it.quality, now.Unix()); err != nil {
					return err
				}
			}
			added++
		}
		return nil
	})
	return added, err
}

func botTiers(rng *rand.Rand, strength float64) game.UpgradeTiers {
	pick := func() int {
		base := strength*float64(game.MaxTier) + rng.NormFloat64()*0.6
		t := int(base + 0.5)
		if t < 0 {
			t = 0
		}
		if t > game.MaxTier {
			t = game.MaxTier
		}
		return t
	}
	return game.UpgradeTiers{
		Engine: pick(), Turbo: pick(), Tires: pick(), Suspension: pick(), Weight: pick(),
	}
}

// botSetup gives a rival a sheet of its own.
//
// Without this every bot raced on factory settings, which quietly handed any
// player who tunes a free edge on every single race — and the edge is worth
// about 5%, so it was not a small gift. It also made the ladder dishonest:
// the whole conceit is that a rival is another player's saved build, and no
// real player leaves the car exactly as it left the workshop.
//
// A bot is set up for *something* rather than for the road it happens to be
// racing on, which is exactly the position a human is in when they cannot be
// bothered to re-tune. So the player's edge survives — it is just earned by
// reading tonight's pass rather than granted by the rivals being asleep.
//
// Weak bots barely touch the car; the top of the mountain runs committed
// sheets, which is also where the game's own crash model starts to bite them.
func botSetup(rng *rand.Rand, strength float64, tiers game.UpgradeTiers) game.Setup {
	if strength < 0.22 {
		return game.Setup{} // still figuring out which end the engine is in
	}
	// How far from factory this driver is willing to go, as a fraction of the
	// axis. Nobody sets a car up in notches, so neither do they: a rival runs
	// 88mm because that is where they stopped turning the collar.
	reach := 1.0
	if strength > 0.62 {
		reach = 2.0
	}
	// How many axes they have actually thought about.
	touched := 2 + int(strength*4)

	var s game.Setup
	order := rng.Perm(len(game.SetupAxes))
	for i, idx := range order {
		if i >= touched {
			break
		}
		axis := game.SetupAxes[idx]
		if !game.AxisUnlocked(axis, tiers) {
			continue
		}
		v := axis.Quantize((rng.Float64()*2 - 1) * reach)
		s, _ = s.With(axis.ID, v)
	}
	// Whatever they set, they cannot adjust a part they have not fitted.
	return game.LegalSetup(s, tiers)
}

func botStats(rng *rand.Rand, level int, strength float64) game.DriverStats {
	st := game.StartingDriverStats()
	points := game.TotalStatPoints(level)
	// Bots specialise: pick a favoured stat and dump extra into it.
	favoured := rng.IntN(len(game.DriverStatNames))
	for i := 0; i < points; i++ {
		idx := rng.IntN(len(game.DriverStatNames))
		if rng.Float64() < 0.35 {
			idx = favoured
		}
		st, _ = st.AddTo(game.DriverStatNames[idx], 1)
	}
	return st
}

func botChassis(rng *rand.Rand, strength float64) string {
	// Weak bots drive the starter car; the top of the ladder is varied.
	if strength < 0.25 {
		return game.StarterChassis
	}
	pool := game.Chassis
	upper := int(strength*float64(len(pool))) + 1
	if upper > len(pool) {
		upper = len(pool)
	}
	return pool[rng.IntN(upper)].ID
}

type seedItem struct {
	itemID  string
	quality float64
}

func botItems(rng *rand.Rand, strength float64) []seedItem {
	// Number of equipped items scales with how far up the mountain they are.
	n := int(strength * 5)
	if n == 0 && rng.Float64() < strength*2 {
		n = 1
	}
	usedSlot := map[string]bool{}
	out := make([]seedItem, 0, n)
	for i := 0; i < n; i++ {
		// quality of stock rises steeply near the top of the ladder
		d, ok := game.RollDrop(rng, 1.0, clamp01(strength*1.15-0.1))
		if !ok {
			continue
		}
		def, ok := game.ItemByID(d.ItemID)
		if !ok || usedSlot[def.Slot] {
			continue
		}
		usedSlot[def.Slot] = true
		out = append(out, seedItem{itemID: d.ItemID, quality: d.Quality})
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
