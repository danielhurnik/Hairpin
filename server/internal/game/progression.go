package game

import (
	"math"
	"math/rand/v2"
	"time"
)

// ---------------------------------------------------------------------------
// Driver levels
// ---------------------------------------------------------------------------

const (
	StartingStatValue = 10
	PointsPerLevel    = 3
	MaxDriverLevel    = 60
)

// StartingDriverStats is where every new driver begins.
func StartingDriverStats() DriverStats {
	return DriverStats{
		Bravery: StartingStatValue, Intelligence: StartingStatValue,
		Reflexes: StartingStatValue, Consistency: StartingStatValue,
		Stamina: StartingStatValue,
	}
}

// XPToNext is the XP required to go from `level` to `level+1`.
func XPToNext(level int) int64 {
	if level < 1 {
		level = 1
	}
	if level >= MaxDriverLevel {
		return 0
	}
	return int64(math.Round(100 * math.Pow(float64(level), 1.6)))
}

// ApplyXP folds earned XP into a level, returning the new level, leftover XP
// and how many stat points were unlocked.
func ApplyXP(level int, xp, gained int64) (newLevel int, newXP int64, points int) {
	level = max(level, 1)
	xp += gained
	for level < MaxDriverLevel {
		need := XPToNext(level)
		if xp < need {
			break
		}
		xp -= need
		level++
		points += PointsPerLevel
	}
	if level >= MaxDriverLevel {
		xp = 0
	}
	return level, xp, points
}

// TotalStatPoints is the number of points a driver at this level has been
// granted in total. Used to validate allocations server-side.
func TotalStatPoints(level int) int {
	if level < 1 {
		level = 1
	}
	return (level - 1) * PointsPerLevel
}

// ---------------------------------------------------------------------------
// Races per day
// ---------------------------------------------------------------------------

const (
	FreeRacesPerDay = 5
	ExtraRaceRubies = 15
	MaxExtraRaces   = 20 // per day, so whales cannot climb infinitely in an hour
)

// NextDailyReset is the next UTC midnight after t.
func NextDailyReset(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
}

// ---------------------------------------------------------------------------
// Job economics
// ---------------------------------------------------------------------------

// RushCost is the ruby price to finish a job with `remaining` left to run.
// Roughly one ruby per ten minutes, always at least one.
func RushCost(remaining time.Duration) int64 {
	if remaining <= 0 {
		return 0
	}
	c := int64(math.Ceil(remaining.Minutes() / 10))
	if c < 1 {
		c = 1
	}
	return c
}

// ---------------------------------------------------------------------------
// Race rewards
// ---------------------------------------------------------------------------

// RaceReward is what a finished race pays out.
type RaceReward struct {
	Cash int64 `json:"cash"`
	XP   int64 `json:"xp"`
}

// RaceRewardFor scales payout with the opponent's ladder rank — beating
// someone near the top is worth far more than farming the bottom.
func RaceRewardFor(won bool, opponentRank int, driverLevel int) RaceReward {
	if opponentRank < 1 {
		opponentRank = 1
	}
	// Prestige runs from ~1.0 at the bottom of the ladder to ~4.0 at rank 1.
	prestige := 1 + 3/math.Sqrt(float64(opponentRank))
	levelScale := math.Pow(float64(max(driverLevel, 1)), 1.35)

	base := 120 * levelScale * prestige
	xp := 40 * levelScale
	if !won {
		base *= 0.30
		xp *= 0.35
	}
	return RaceReward{Cash: int64(base), XP: int64(xp)}
}

// ---------------------------------------------------------------------------
// Ladder movement
// ---------------------------------------------------------------------------

// LadderBandAbove is how far up the ladder a challenger can be matched.
const LadderBandAbove = 6

// ---------------------------------------------------------------------------
// Drops
// ---------------------------------------------------------------------------

// Drop is a rolled item award.
type Drop struct {
	ItemID  string  `json:"item_id"`
	Quality float64 `json:"quality"`
}

// RollDrop decides whether an item drops and which one. `chance` is the
// probability of any drop; `quality` (0..1) shifts the rarity distribution.
// Always called server-side with a server-chosen seed.
func RollDrop(rng *rand.Rand, chance, quality float64) (Drop, bool) {
	if rng.Float64() >= chance {
		return Drop{}, false
	}
	rarity := rollRarity(rng, quality)

	pool := make([]ItemDef, 0, len(Items))
	for _, it := range Items {
		if it.Rarity == rarity {
			pool = append(pool, it)
		}
	}
	if len(pool) == 0 {
		return Drop{}, false
	}
	pick := pool[rng.IntN(len(pool))]

	// Quality roll makes two copies of the same item meaningfully different.
	q := 0.88 + rng.Float64()*0.30
	return Drop{ItemID: pick.ID, Quality: math.Round(q*100) / 100}, true
}

func rollRarity(rng *rand.Rand, quality float64) Rarity {
	weights := RarityWeights(quality)
	order := []Rarity{Common, Uncommon, Rare, Epic, Legendary}
	var total float64
	for _, r := range order {
		total += weights[r]
	}
	x := rng.Float64() * total
	for _, r := range order {
		x -= weights[r]
		if x <= 0 {
			return r
		}
	}
	return Common
}

// ItemBonus folds an owned item's definition and quality roll into stats.
func ItemBonus(def ItemDef, quality float64) EquipBonus {
	q := quality
	if q <= 0 {
		q = 1
	}
	scale := func(v float64) float64 { return math.Round(v*q*10) / 10 }
	return EquipBonus{
		Driver: DriverStats{
			Bravery:      scale(def.Driver.Bravery),
			Intelligence: scale(def.Driver.Intelligence),
			Reflexes:     scale(def.Driver.Reflexes),
			Consistency:  scale(def.Driver.Consistency),
			Stamina:      scale(def.Driver.Stamina),
		},
		Power:    scale(def.Power),
		Grip:     scale(def.Grip),
		Handling: scale(def.Handling),
		Braking:  scale(def.Braking),
		WeightKg: scale(def.WeightKg),
	}
}

// SumBonuses merges equipment bonuses.
func SumBonuses(bs ...EquipBonus) EquipBonus {
	var out EquipBonus
	for _, b := range bs {
		out.Driver = out.Driver.Add(b.Driver)
		out.Power += b.Power
		out.Grip += b.Grip
		out.Handling += b.Handling
		out.Braking += b.Braking
		out.WeightKg += b.WeightKg
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
