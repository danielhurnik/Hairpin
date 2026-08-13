package game

import "math"

// Drivetrain affects how a chassis behaves on different segment kinds.
type Drivetrain string

const (
	FR  Drivetrain = "FR"
	FF  Drivetrain = "FF"
	MR  Drivetrain = "MR"
	AWD Drivetrain = "AWD"
)

// DriverStats are the human half of the equation. Every value is a raw point
// total; ReferenceDriverStat is what a "average" driver looks like.
type DriverStats struct {
	Bravery      float64 `json:"bravery"`
	Intelligence float64 `json:"intelligence"`
	Reflexes     float64 `json:"reflexes"`
	Consistency  float64 `json:"consistency"`
	Stamina      float64 `json:"stamina"`
}

func (d DriverStats) Add(o DriverStats) DriverStats {
	return DriverStats{
		Bravery:      d.Bravery + o.Bravery,
		Intelligence: d.Intelligence + o.Intelligence,
		Reflexes:     d.Reflexes + o.Reflexes,
		Consistency:  d.Consistency + o.Consistency,
		Stamina:      d.Stamina + o.Stamina,
	}
}

func (d DriverStats) Total() float64 {
	return d.Bravery + d.Intelligence + d.Reflexes + d.Consistency + d.Stamina
}

// Get returns a stat by its wire name, and whether the name was valid.
func (d DriverStats) Get(name string) (float64, bool) {
	switch name {
	case "bravery":
		return d.Bravery, true
	case "intelligence":
		return d.Intelligence, true
	case "reflexes":
		return d.Reflexes, true
	case "consistency":
		return d.Consistency, true
	case "stamina":
		return d.Stamina, true
	}
	return 0, false
}

// AddTo returns a copy with `points` added to the named stat.
func (d DriverStats) AddTo(name string, points float64) (DriverStats, bool) {
	switch name {
	case "bravery":
		d.Bravery += points
	case "intelligence":
		d.Intelligence += points
	case "reflexes":
		d.Reflexes += points
	case "consistency":
		d.Consistency += points
	case "stamina":
		d.Stamina += points
	default:
		return d, false
	}
	return d, true
}

// DriverStatNames is the canonical ordering used by the client.
var DriverStatNames = []string{"bravery", "intelligence", "reflexes", "consistency", "stamina"}

// CarStats are the derived, race-ready numbers for a build. Nothing here is
// stored; it is always recomputed from chassis + upgrade tiers + equipment so
// a tampered client can never inject a number.
type CarStats struct {
	Power      float64    `json:"power"`       // crank hp
	Weight     float64    `json:"weight"`      // kg
	PowerToWt  float64    `json:"power_to_wt"` // hp per tonne
	Grip       float64    `json:"grip"`
	Handling   float64    `json:"handling"`
	Braking    float64    `json:"braking"`
	Drivetrain Drivetrain `json:"drivetrain"`
	// TurboLag adds variance: big turbos are fast but spiky.
	TurboLag float64 `json:"turbo_lag"`
}

// Reference values. A stock AE-Series Coupe with an average driver sits at
// roughly 100 rating, which keeps the numbers on screen human-readable.
const (
	ReferencePowerToWt  = 138.0
	ReferenceGrip       = 50.0
	ReferenceHandling   = 72.0
	ReferenceBraking    = 58.0
	ReferenceDriverStat = 12.0
)

// Build is everything the server needs to simulate one competitor.
type Build struct {
	Car    CarStats    `json:"car"`
	Driver DriverStats `json:"driver"`
	// Setup is deliberately not serialised. A rival's sheet is theirs — being
	// able to read it would turn tuning into copying, and the opponent view is
	// sent to the client in full.
	Setup Setup `json:"-"`
	// SetupNorm keeps the setup a pure trade for THIS car. Set by WithSetup;
	// never assign it by hand.
	SetupNorm float64 `json:"-"`
}

// UpgradeTiers holds the purchased tier (0..MaxTier) for each category.
type UpgradeTiers struct {
	Engine     int `json:"engine"`
	Turbo      int `json:"turbo"`
	Tires      int `json:"tires"`
	Suspension int `json:"suspension"`
	Weight     int `json:"weight"`
}

func (u UpgradeTiers) Get(cat string) (int, bool) {
	switch cat {
	case CatEngine:
		return u.Engine, true
	case CatTurbo:
		return u.Turbo, true
	case CatTires:
		return u.Tires, true
	case CatSuspension:
		return u.Suspension, true
	case CatWeight:
		return u.Weight, true
	}
	return 0, false
}

func (u UpgradeTiers) With(cat string, tier int) (UpgradeTiers, bool) {
	switch cat {
	case CatEngine:
		u.Engine = tier
	case CatTurbo:
		u.Turbo = tier
	case CatTires:
		u.Tires = tier
	case CatSuspension:
		u.Suspension = tier
	case CatWeight:
		u.Weight = tier
	default:
		return u, false
	}
	return u, true
}

// EquipBonus is the flat contribution of all equipped items.
type EquipBonus struct {
	Driver   DriverStats
	Power    float64
	Grip     float64
	Handling float64
	Braking  float64
	WeightKg float64 // negative = lighter
}

// ComputeCar folds chassis, upgrade tiers and equipment into race-ready stats.
func ComputeCar(chassisID string, tiers UpgradeTiers, eq EquipBonus) CarStats {
	ch, ok := ChassisByID(chassisID)
	if !ok {
		ch = Chassis[0]
	}
	power := ch.Power + EngineTable.Power[clampTier(tiers.Engine)] + TurboTable.Power[clampTier(tiers.Turbo)] + eq.Power
	weight := ch.Weight - WeightTable.WeightKg[clampTier(tiers.Weight)] + eq.WeightKg
	if weight < 500 {
		weight = 500
	}
	grip := ch.Grip + TireTable.Grip[clampTier(tiers.Tires)] + SuspensionTable.Grip[clampTier(tiers.Suspension)] + eq.Grip
	handling := ch.Handling + SuspensionTable.Handling[clampTier(tiers.Suspension)] + eq.Handling
	braking := ch.Braking + TireTable.Braking[clampTier(tiers.Tires)] + eq.Braking

	return CarStats{
		Power:      round1(power),
		Weight:     round1(weight),
		PowerToWt:  round1(power / weight * 1000),
		Grip:       round1(grip),
		Handling:   round1(handling),
		Braking:    round1(braking),
		Drivetrain: ch.Drivetrain,
		TurboLag:   TurboTable.Lag[clampTier(tiers.Turbo)],
	}
}

func clampTier(t int) int {
	if t < 0 {
		return 0
	}
	if t > MaxTier {
		return MaxTier
	}
	return t
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
