package game

import (
	"math/rand/v2"
	"time"
)

// K's Office is the back room where a man with no surname sells things that
// are supposed to be undroppable. Stock rotates daily and is deterministic
// per player per day, so refreshing the app cannot reroll it.

const KOfficeStockSize = 6

type ShopOffer struct {
	// OfferID is stable for a given player+day+slot, so a purchase request
	// cannot be replayed against a different item.
	OfferID   string  `json:"offer_id"`
	ItemID    string  `json:"item_id"`
	Name      string  `json:"name"`
	Slot      string  `json:"slot"`
	Rarity    Rarity  `json:"rarity"`
	Flavour   string  `json:"flavour"`
	Quality   float64 `json:"quality"`
	PriceCash int64   `json:"price_cash,omitempty"`
	PriceRuby int64   `json:"price_rubies,omitempty"`
	SoldOut   bool    `json:"sold_out"`
}

// ShopDayKey is the rotation key: YYYY-MM-DD in UTC.
func ShopDayKey(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

// RollShop generates today's K's Office stock for one player. The seed is
// derived from the player and the day, never from the request, so the client
// cannot influence what appears.
func RollShop(seed uint64, quality float64) []ShopOffer {
	rng := rand.New(rand.NewPCG(seed, seed^0xA5A5A5A5DEADBEEF))

	tradeable := make([]ItemDef, 0, len(Items))
	for _, it := range Items {
		if it.Tradeable {
			tradeable = append(tradeable, it)
		}
	}

	offers := make([]ShopOffer, 0, KOfficeStockSize)
	used := map[string]bool{}

	for slot := 0; slot < KOfficeStockSize; slot++ {
		// The last two slots are the premium shelf: rarer stock, rubies only.
		premium := slot >= KOfficeStockSize-2
		q := quality
		if premium {
			q = clamp(quality+0.45, 0, 1)
		}

		var pick ItemDef
		for attempt := 0; attempt < 24; attempt++ {
			rarity := rollRarity(rng, q)
			pool := tradeable[:0:0]
			for _, it := range tradeable {
				if it.Rarity == rarity && !used[it.ID] {
					pool = append(pool, it)
				}
			}
			if len(pool) > 0 {
				pick = pool[rng.IntN(len(pool))]
				break
			}
		}
		if pick.ID == "" {
			continue
		}
		used[pick.ID] = true

		itemQ := 0.9 + rng.Float64()*0.28
		itemQ = float64(int(itemQ*100)) / 100

		o := ShopOffer{
			OfferID: shopOfferID(seed, slot),
			ItemID:  pick.ID,
			Name:    pick.Name,
			Slot:    pick.Slot,
			Rarity:  pick.Rarity,
			Flavour: pick.Flavour,
			Quality: itemQ,
		}
		price := ShopPrice(pick, itemQ)
		if premium || pick.Rarity == Legendary {
			// Rubies are priced off the cash value at a deliberately poor rate;
			// the shop is a shortcut, not a discount.
			o.PriceRuby = price/900 + 25
		} else {
			o.PriceCash = price
		}
		offers = append(offers, o)
	}
	return offers
}

func shopOfferID(seed uint64, slot int) string {
	const hex = "0123456789abcdef"
	v := seed ^ (uint64(slot+1) * 0x9E3779B97F4A7C15)
	b := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		b[i] = hex[v&0xF]
		v >>= 4
	}
	return string(b)
}
