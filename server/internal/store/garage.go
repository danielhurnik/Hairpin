package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// UpgradeOption is one row of the garage screen.
type UpgradeOption struct {
	Category   string `json:"category"`
	Label      string `json:"label"`
	Tier       int    `json:"tier"`
	MaxTier    int    `json:"max_tier"`
	TierName   string `json:"tier_name"`
	NextName   string `json:"next_tier_name,omitempty"`
	NextCost   int64  `json:"next_cost,omitempty"`
	Affordable bool   `json:"affordable"`
	Maxed      bool   `json:"maxed"`
	// RatingDelta is what buying the next tier would do to the overall rating.
	RatingDelta float64 `json:"rating_delta,omitempty"`
}

// GarageOptions prices every upgrade and shows what each one would actually
// buy in rating terms — computed server-side from the same model the race uses.
func (s *Store) GarageOptions(ctx context.Context, p *Player, current game.Build) []UpgradeOption {
	base := game.OverallRating(current)
	out := make([]UpgradeOption, 0, len(game.UpgradeCategories))

	for _, cat := range game.UpgradeCategories {
		tier, _ := p.Tiers.Get(cat)
		opt := UpgradeOption{
			Category: cat,
			Label:    game.UpgradeLabel(cat),
			Tier:     tier,
			MaxTier:  game.MaxTier,
			TierName: game.UpgradeTierName(cat, tier),
		}
		if tier >= game.MaxTier {
			opt.Maxed = true
			out = append(out, opt)
			continue
		}
		cost, _ := game.UpgradeCost(cat, tier+1)
		opt.NextCost = cost
		opt.NextName = game.UpgradeTierName(cat, tier+1)
		opt.Affordable = p.Cash >= cost

		next, _ := p.Tiers.With(cat, tier+1)
		preview := current
		preview.Car = game.ComputeCar(p.ChassisID, next, carBonusOnly(current, p))
		opt.RatingDelta = round2(game.OverallRating(preview) - base)

		out = append(out, opt)
	}
	return out
}

// carBonusOnly recovers the equipment contribution to the car so a preview can
// be recomputed at a different tier without re-reading the database.
func carBonusOnly(current game.Build, p *Player) game.EquipBonus {
	stock := game.ComputeCar(p.ChassisID, p.Tiers, game.EquipBonus{})
	return game.EquipBonus{
		Power:    current.Car.Power - stock.Power,
		Grip:     current.Car.Grip - stock.Grip,
		Handling: current.Car.Handling - stock.Handling,
		Braking:  current.Car.Braking - stock.Braking,
		WeightKg: current.Car.Weight - stock.Weight,
	}
}

// BuyUpgrade purchases the next tier in a category. Price and tier validity
// are read from the server's tables, never from the request.
func (s *Store) BuyUpgrade(ctx context.Context, playerID, category string) (*Player, int64, error) {
	if _, ok := game.UpgradeCost(category, 1); !ok {
		return nil, 0, ErrInvalid
	}
	var out *Player
	var paid int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		tier, ok := p.Tiers.Get(category)
		if !ok {
			return ErrInvalid
		}
		if tier >= game.MaxTier {
			return ErrMaxTier
		}
		cost, ok := game.UpgradeCost(category, tier+1)
		if !ok {
			return ErrInvalid
		}
		if p.Cash < cost {
			return ErrInsufficient
		}
		// Column name is derived from a fixed whitelist, never from input.
		col, ok := tierColumn(category)
		if !ok {
			return ErrInvalid
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE players SET cash = cash - ?, %s = ?, updated_at = ? WHERE id = ? AND cash >= ?`, col),
			cost, tier+1, s.Now().Unix(), playerID, cost); err != nil {
			return err
		}
		paid = cost
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, paid, err
}

func tierColumn(cat string) (string, bool) {
	switch cat {
	case game.CatEngine:
		return "tier_engine", true
	case game.CatTurbo:
		return "tier_turbo", true
	case game.CatTires:
		return "tier_tires", true
	case game.CatSuspension:
		return "tier_suspension", true
	case game.CatWeight:
		return "tier_weight", true
	}
	return "", false
}

// AllocateStat spends an unspent driver point. The server tracks the budget;
// the client only names a stat.
func (s *Store) AllocateStat(ctx context.Context, playerID, stat string, points int) (*Player, error) {
	if points <= 0 {
		return nil, ErrInvalid
	}
	if _, ok := game.StartingDriverStats().Get(stat); !ok {
		return nil, ErrInvalid
	}
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if p.UnspentPoints < points {
			return ErrInsufficient
		}
		col, ok := statColumn(stat)
		if !ok {
			return ErrInvalid
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE players SET %s = %s + ?, unspent_points = unspent_points - ?, updated_at = ?
				WHERE id = ? AND unspent_points >= ?`, col, col),
			points, points, s.Now().Unix(), playerID, points); err != nil {
			return err
		}
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, err
}

func statColumn(stat string) (string, bool) {
	switch stat {
	case "bravery":
		return "stat_bravery", true
	case "intelligence":
		return "stat_intelligence", true
	case "reflexes":
		return "stat_reflexes", true
	case "consistency":
		return "stat_consistency", true
	case "stamina":
		return "stat_stamina", true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Liveries — cosmetic only
// ---------------------------------------------------------------------------

type LiveryOption struct {
	game.LiveryDef
	Owned    bool `json:"owned"`
	Equipped bool `json:"equipped"`
}

func (s *Store) Liveries(ctx context.Context, p *Player) ([]LiveryOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT livery_id FROM player_liveries WHERE player_id = ?`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owned := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		owned[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]LiveryOption, 0, len(game.Liveries))
	for _, l := range game.Liveries {
		out = append(out, LiveryOption{LiveryDef: l, Owned: owned[l.ID], Equipped: p.LiveryID == l.ID})
	}
	return out, nil
}

func (s *Store) BuyLivery(ctx context.Context, playerID, liveryID string) (*Player, error) {
	def, ok := game.LiveryByID(liveryID)
	if !ok {
		return nil, ErrInvalid
	}
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM player_liveries WHERE player_id = ? AND livery_id = ?`,
			playerID, liveryID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadyOwned
		}
		now := s.Now()
		switch {
		case def.PriceRuby > 0:
			if p.Rubies < def.PriceRuby {
				return ErrInsufficient
			}
			if _, err := tx.ExecContext(ctx, `UPDATE players SET rubies = rubies - ?, updated_at = ? WHERE id = ? AND rubies >= ?`,
				def.PriceRuby, now.Unix(), playerID, def.PriceRuby); err != nil {
				return err
			}
		default:
			if p.Cash < def.PriceCash {
				return ErrInsufficient
			}
			if _, err := tx.ExecContext(ctx, `UPDATE players SET cash = cash - ?, updated_at = ? WHERE id = ? AND cash >= ?`,
				def.PriceCash, now.Unix(), playerID, def.PriceCash); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO player_liveries (player_id, livery_id, acquired_at) VALUES (?, ?, ?)`,
			playerID, liveryID, now.Unix()); err != nil {
			return err
		}
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, err
}

func (s *Store) EquipLivery(ctx context.Context, playerID, liveryID string) error {
	if _, ok := game.LiveryByID(liveryID); !ok {
		return ErrInvalid
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM player_liveries WHERE player_id = ? AND livery_id = ?`,
		playerID, liveryID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	_, err := s.db.ExecContext(ctx, `UPDATE players SET livery_id = ?, updated_at = ? WHERE id = ?`,
		liveryID, s.Now().Unix(), playerID)
	return err
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5*sign(v))) / 100
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}
