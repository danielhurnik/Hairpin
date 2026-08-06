package store

import (
	"context"
	"database/sql"
	"hash/fnv"
	"strings"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// K's Office. Stock is a pure function of (player, day), so it is never stored
// and never trusted from the client: we re-roll it on every request and on
// every purchase, and compare offer IDs.

func shopSeed(playerID, dayKey string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(playerID))
	h.Write([]byte{0})
	h.Write([]byte(dayKey))
	return h.Sum64()
}

// shopQuality gates how good today's stock can be by driver level, so a
// brand-new account cannot buy a legendary on day one.
func shopQuality(level int) float64 {
	return clamp01(float64(level) / 40)
}

type ShopView struct {
	DayKey string           `json:"day_key"`
	Offers []game.ShopOffer `json:"offers"`
	Packs  []game.RubyPack  `json:"ruby_packs"`
	Cash   int64            `json:"cash"`
	Rubies int64            `json:"rubies"`
}

func (s *Store) Shop(ctx context.Context, p *Player) (*ShopView, error) {
	dayKey := game.ShopDayKey(s.Now())
	offers := game.RollShop(shopSeed(p.ID, dayKey), shopQuality(p.Level))

	sold, err := s.soldOffers(ctx, p.ID, dayKey)
	if err != nil {
		return nil, err
	}
	for i := range offers {
		if sold[offers[i].OfferID] {
			offers[i].SoldOut = true
		}
	}
	return &ShopView{DayKey: dayKey, Offers: offers, Packs: game.RubyPacks, Cash: p.Cash, Rubies: p.Rubies}, nil
}

func (s *Store) soldOffers(ctx context.Context, playerID, dayKey string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT offer_id FROM shop_purchases WHERE player_id = ? AND day_key = ?`, playerID, dayKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// BuyOffer purchases one item from K's Office. The price is taken from the
// server's own re-roll of today's stock, never from the request body.
func (s *Store) BuyOffer(ctx context.Context, playerID, offerID string) (*InventoryItem, *Player, error) {
	var item *InventoryItem
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		dayKey := game.ShopDayKey(now)
		offers := game.RollShop(shopSeed(p.ID, dayKey), shopQuality(p.Level))

		var offer *game.ShopOffer
		for i := range offers {
			if offers[i].OfferID == offerID {
				offer = &offers[i]
				break
			}
		}
		if offer == nil {
			return ErrNotFound
		}

		// The primary key on (player_id, offer_id) is what actually prevents
		// double-buying under concurrency; this is just the friendly error.
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM shop_purchases WHERE player_id = ? AND offer_id = ?`,
			playerID, offerID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrOfferPurchased
		}

		switch {
		case offer.PriceRuby > 0:
			if p.Rubies < offer.PriceRuby {
				return ErrInsufficient
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE players SET rubies = rubies - ?, updated_at = ? WHERE id = ? AND rubies >= ?`,
				offer.PriceRuby, now.Unix(), playerID, offer.PriceRuby); err != nil {
				return err
			}
		default:
			if p.Cash < offer.PriceCash {
				return ErrInsufficient
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE players SET cash = cash - ?, updated_at = ? WHERE id = ? AND cash >= ?`,
				offer.PriceCash, now.Unix(), playerID, offer.PriceCash); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO shop_purchases (player_id, offer_id, day_key, created_at) VALUES (?, ?, ?, ?)`,
			playerID, offerID, dayKey, now.Unix()); err != nil {
			if isUniqueViolation(err) {
				return ErrOfferPurchased
			}
			return err
		}

		item, err = insertItem(ctx, tx, playerID,
			game.Drop{ItemID: offer.ItemID, Quality: offer.Quality}, "shop", now)
		if err != nil {
			return err
		}
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return item, out, err
}

// GrantRubies credits an in-app purchase. The caller is responsible for having
// validated the receipt with Apple/Google first — see the handler.
// The UNIQUE constraint on receipt is the replay defence.
func (s *Store) GrantRubies(ctx context.Context, playerID, packID, receipt string) (*Player, int64, error) {
	pack, ok := game.RubyPackByID(packID)
	if !ok {
		return nil, 0, ErrInvalid
	}
	total := pack.Rubies + pack.Bonus
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ruby_purchases (player_id, pack_id, receipt, rubies, created_at) VALUES (?, ?, ?, ?, ?)`,
			playerID, packID, receipt, total, now.Unix()); err != nil {
			if isUniqueViolation(err) {
				return ErrAlreadyOwned // receipt already redeemed
			}
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE players SET rubies = rubies + ?, updated_at = ? WHERE id = ?`,
			total, now.Unix(), playerID); err != nil {
			return err
		}
		var err error
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, total, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique constraint") || strings.Contains(s, "constraint failed")
}
