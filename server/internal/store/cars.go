package store

import (
	"context"
	"database/sql"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// CarOption is one row of the showroom: a chassis, whether it is already in
// the garage, and — if it is not — exactly what buying it would do.
type CarOption struct {
	game.ChassisDef
	Owned      bool `json:"owned"`
	Active     bool `json:"active"`
	Buyable    bool `json:"buyable"`
	Affordable bool `json:"affordable"`
	LevelMet   bool `json:"level_met"`
	// Tiers is the build this car is carrying, for cars already owned.
	Tiers game.UpgradeTiers `json:"upgrades"`
	// Rating is what this car would race at right now, with the player's
	// current driver and equipment. Computed with the model the race uses, so
	// the showroom cannot promise something the mountain will not deliver.
	Rating float64 `json:"rating"`
	// RatingDelta is Rating minus the active car's rating. Negative is honest
	// and stays visible: a stock supercar really is slower than a built hatch.
	RatingDelta float64 `json:"rating_delta"`
}

// Cars lists every chassis in the game against what the player owns. Prices,
// gates and ratings are all server-side; the client renders the answer.
func (s *Store) Cars(ctx context.Context, p *Player, current game.Build) ([]CarOption, error) {
	owned, err := ownedCars(ctx, s.db, p.ID)
	if err != nil {
		return nil, err
	}
	// Equipment travels with the driver, not the car, so the same bonus
	// applies to every car being previewed.
	eq := carBonusOnly(current, p)
	base := game.OverallRating(current)

	out := make([]CarOption, 0, len(game.Chassis))
	for _, ch := range game.Chassis {
		tiers, have := owned[ch.ID]
		preview := current
		preview.Car = game.ComputeCar(ch.ID, tiers, eq)
		rating := game.OverallRating(preview)

		out = append(out, CarOption{
			ChassisDef:  ch,
			Owned:       have,
			Active:      ch.ID == p.ChassisID,
			Buyable:     !have && ch.Price > 0,
			Affordable:  p.Cash >= ch.Price,
			LevelMet:    p.Level >= ch.MinLevel,
			Tiers:       tiers,
			Rating:      round2(rating),
			RatingDelta: round2(rating - base),
		})
	}
	return out, nil
}

// ownedCars maps chassis ID to that car's tiers. A car absent from the map is
// one the player does not own — there is no other ownership record.
func ownedCars(ctx context.Context, q queryer, playerID string) (map[string]game.UpgradeTiers, error) {
	rows, err := q.QueryContext(ctx, `SELECT chassis_id, tier_engine, tier_turbo, tier_tires,
		tier_suspension, tier_weight FROM player_cars WHERE player_id = ?`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]game.UpgradeTiers{}
	for rows.Next() {
		var id string
		var t game.UpgradeTiers
		if err := rows.Scan(&id, &t.Engine, &t.Turbo, &t.Tires, &t.Suspension, &t.Weight); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}

// BuyChassis puts a new car in the garage. The price and the level gate are
// read from the server's tables; the request only names a chassis.
//
// The new car arrives stock. That is the design: a chassis purchase is the
// start of a build, not a free copy of the one you already finished.
func (s *Store) BuyChassis(ctx context.Context, playerID, chassisID string) (*Player, int64, error) {
	def, ok := game.ChassisByID(chassisID)
	if !ok || def.Price <= 0 {
		return nil, 0, ErrInvalid
	}
	var out *Player
	var paid int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if p.Level < def.MinLevel {
			return ErrLevelLocked
		}
		if p.Cash < def.Price {
			return ErrInsufficient
		}
		now := s.Now()
		if err := execGuarded(ctx, tx, ErrInsufficient,
			`UPDATE players SET cash = cash - ?, updated_at = ? WHERE id = ? AND cash >= ?`,
			def.Price, now.Unix(), playerID, def.Price); err != nil {
			return err
		}
		// The primary key is what rejects a second purchase — checking first
		// and inserting after would leave a window between the two.
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO player_cars
			(player_id, chassis_id, acquired_at) VALUES (?, ?, ?)`,
			playerID, chassisID, now.Unix())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			// Already owned. Rolling back is what un-charges them.
			return ErrAlreadyOwned
		}
		paid = def.Price
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return out, paid, nil
}

// SetActiveCar switches which owned car races. Free, reversible, and the only
// thing it changes is the pointer — each car keeps its own build.
func (s *Store) SetActiveCar(ctx context.Context, playerID, chassisID string) (*Player, error) {
	if _, ok := game.ChassisByID(chassisID); !ok {
		return nil, ErrInvalid
	}
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM player_cars WHERE player_id = ? AND chassis_id = ?`,
			playerID, chassisID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE players SET chassis_id = ?, updated_at = ? WHERE id = ?`,
			chassisID, s.Now().Unix(), playerID); err != nil {
			return err
		}
		var err error
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, err
}
