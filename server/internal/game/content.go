package game

import "time"

// ---------------------------------------------------------------------------
// Chassis
// ---------------------------------------------------------------------------

type ChassisDef struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Drivetrain Drivetrain `json:"drivetrain"`
	Power      float64    `json:"power"`
	Weight     float64    `json:"weight"`
	Grip       float64    `json:"grip"`
	Handling   float64    `json:"handling"`
	Braking    float64    `json:"braking"`
}

// Every player starts in the AE-Series Coupe. The rest exist so the ladder
// bots have visual and mechanical variety; buying a chassis is post-M1.
var Chassis = []ChassisDef{
	{ID: "ae_coupe", Name: "AE-Series Coupe", Drivetrain: FR, Power: 130, Weight: 940, Grip: 46, Handling: 72, Braking: 58},
	{ID: "eg_hatch", Name: "EG-Series Hatch", Drivetrain: FF, Power: 170, Weight: 1040, Grip: 52, Handling: 66, Braking: 56},
	{ID: "s_roadster", Name: "S-Series Roadster", Drivetrain: FR, Power: 205, Weight: 1180, Grip: 55, Handling: 68, Braking: 60},
	{ID: "fc_rotary", Name: "FC-Series Rotary", Drivetrain: FR, Power: 215, Weight: 1260, Grip: 56, Handling: 63, Braking: 62},
	{ID: "mr_mid", Name: "MR-Series Mid-Engine", Drivetrain: MR, Power: 200, Weight: 1100, Grip: 58, Handling: 74, Braking: 61},
	{ID: "gc_awd", Name: "GC-Series AWD", Drivetrain: AWD, Power: 276, Weight: 1270, Grip: 68, Handling: 58, Braking: 66},
	{ID: "r_gt", Name: "R-Series GT", Drivetrain: AWD, Power: 280, Weight: 1560, Grip: 72, Handling: 50, Braking: 70},
}

const StarterChassis = "ae_coupe"

func ChassisByID(id string) (ChassisDef, bool) {
	for _, c := range Chassis {
		if c.ID == id {
			return c, true
		}
	}
	return ChassisDef{}, false
}

// ---------------------------------------------------------------------------
// Upgrades — 5 categories, 5 tiers each (index 0 = stock)
// ---------------------------------------------------------------------------

const MaxTier = 5

const (
	CatEngine     = "engine"
	CatTurbo      = "turbo"
	CatTires      = "tires"
	CatSuspension = "suspension"
	CatWeight     = "weight"
)

var UpgradeCategories = []string{CatEngine, CatTurbo, CatTires, CatSuspension, CatWeight}

type upgradeTable struct {
	Label    string
	Cost     [MaxTier + 1]int64 // cash cost to REACH this tier
	Power    [MaxTier + 1]float64
	Grip     [MaxTier + 1]float64
	Handling [MaxTier + 1]float64
	Braking  [MaxTier + 1]float64
	WeightKg [MaxTier + 1]float64
	Lag      [MaxTier + 1]float64
	Names    [MaxTier + 1]string
}

var EngineTable = upgradeTable{
	Label: "Engine",
	Cost:  [6]int64{0, 250, 900, 3000, 9000, 26000},
	Power: [6]float64{0, 12, 28, 50, 80, 120},
	Names: [6]string{"Stock", "Intake & Exhaust", "Ported Head", "Forged Internals", "Individual Throttle Bodies", "Full Race Build"},
}

var TurboTable = upgradeTable{
	Label: "Turbo",
	Cost:  [6]int64{0, 400, 1400, 4500, 13000, 36000},
	Power: [6]float64{0, 20, 45, 80, 130, 200},
	Lag:   [6]float64{0, 0.004, 0.008, 0.013, 0.018, 0.025},
	Names: [6]string{"Naturally Aspirated", "Bolt-On Snail", "Ball Bearing Unit", "Twin-Scroll", "Big Single", "Anti-Lag Monster"},
}

var TireTable = upgradeTable{
	Label:   "Tires",
	Cost:    [6]int64{0, 180, 650, 2200, 6800, 19000},
	Grip:    [6]float64{0, 6, 13, 21, 31, 43},
	Braking: [6]float64{0, 1.5, 3.5, 6, 9, 13},
	Names:   [6]string{"Hard Compound", "Sport Touring", "Semi-Slick", "Soft Semi-Slick", "Cut Slick", "Mountain Spec"},
}

var SuspensionTable = upgradeTable{
	Label:    "Suspension",
	Cost:     [6]int64{0, 220, 800, 2600, 7800, 22000},
	Handling: [6]float64{0, 5, 11, 18, 27, 38},
	Grip:     [6]float64{0, 2, 4, 7, 10, 14},
	Names:    [6]string{"Worn OEM", "Lowering Springs", "Adjustable Coilovers", "Pillow-Ball Arms", "2-Way Dampers", "Pro Circuit Setup"},
}

var WeightTable = upgradeTable{
	Label:    "Weight Reduction",
	Cost:     [6]int64{0, 200, 750, 2400, 7200, 20000},
	WeightKg: [6]float64{0, 30, 65, 105, 155, 215},
	Names:    [6]string{"Full Interior", "Stripped Boot", "No Rear Seats", "Carbon Panels", "Lexan Windows", "Bare Shell"},
}

func tableFor(cat string) (upgradeTable, bool) {
	switch cat {
	case CatEngine:
		return EngineTable, true
	case CatTurbo:
		return TurboTable, true
	case CatTires:
		return TireTable, true
	case CatSuspension:
		return SuspensionTable, true
	case CatWeight:
		return WeightTable, true
	}
	return upgradeTable{}, false
}

// UpgradeCost is the cash price to move from tier-1 to tier.
func UpgradeCost(cat string, tier int) (int64, bool) {
	t, ok := tableFor(cat)
	if !ok || tier < 1 || tier > MaxTier {
		return 0, false
	}
	return t.Cost[tier], true
}

// UpgradeTierName is the flavour label shown in the garage.
func UpgradeTierName(cat string, tier int) string {
	t, ok := tableFor(cat)
	if !ok {
		return ""
	}
	return t.Names[clampTier(tier)]
}

func UpgradeLabel(cat string) string {
	t, _ := tableFor(cat)
	return t.Label
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

type JobDef struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Flavour    string        `json:"flavour"`
	Tier       int           `json:"tier"`
	Duration   time.Duration `json:"-"`
	DurationS  int           `json:"duration_seconds"`
	Cash       int64         `json:"cash"`
	XP         int64         `json:"xp"`
	MinLevel   int           `json:"min_level"`
	DropChance float64       `json:"drop_chance"`
	// DropTable weights how good a drop is when one lands.
	DropQuality float64 `json:"-"`
}

var Jobs = []JobDef{
	{
		ID: "tofu_run", Name: "Tofu Delivery Run", Tier: 1,
		Flavour:  "A crate in the boot, a cup of water on the dash. Don't spill it.",
		Duration: 30 * time.Second, Cash: 45, XP: 12, MinLevel: 1,
		DropChance: 0.03, DropQuality: 0.15,
	},
	{
		ID: "pizza_shift", Name: "Late Shift Pizza", Tier: 2,
		Flavour:  "Thirty minutes or it's free. The pass is faster than the highway.",
		Duration: 3 * time.Minute, Cash: 260, XP: 55, MinLevel: 3,
		DropChance: 0.06, DropQuality: 0.3,
	},
	{
		ID: "courier_night", Name: "Night Courier Contract", Tier: 3,
		Flavour:  "Sealed envelope, no questions, three prefectures before sunrise.",
		Duration: 15 * time.Minute, Cash: 1100, XP: 210, MinLevel: 6,
		DropChance: 0.11, DropQuality: 0.5,
	},
	{
		ID: "parts_haul", Name: "Parts Haul from the Docks", Tier: 4,
		Flavour:  "Half a container of grey-import bits. The van drives itself home.",
		Duration: 1 * time.Hour, Cash: 4200, XP: 780, MinLevel: 10,
		DropChance: 0.18, DropQuality: 0.72,
	},
	{
		ID: "shop_shift", Name: "Weekend at the Tuning Shop", Tier: 5,
		Flavour:  "Sweep the floor, hold the light, learn everything.",
		Duration: 4 * time.Hour, Cash: 18000, XP: 3200, MinLevel: 15,
		DropChance: 0.27, DropQuality: 1.0,
	},
}

func JobByID(id string) (JobDef, bool) {
	for _, j := range Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return JobDef{}, false
}

// ---------------------------------------------------------------------------
// Items
// ---------------------------------------------------------------------------

type Rarity string

const (
	Common    Rarity = "common"
	Uncommon  Rarity = "uncommon"
	Rare      Rarity = "rare"
	Epic      Rarity = "epic"
	Legendary Rarity = "legendary"
)

var RarityOrder = map[Rarity]int{Common: 0, Uncommon: 1, Rare: 2, Epic: 3, Legendary: 4}

// Slots. One item equipped per slot.
const (
	SlotHead  = "head"
	SlotHands = "hands"
	SlotFeet  = "feet"
	SlotSuit  = "suit"
	SlotMind  = "mind"
	SlotCharm = "charm"
	SlotCar   = "car"
)

var Slots = []string{SlotHead, SlotHands, SlotFeet, SlotSuit, SlotMind, SlotCharm, SlotCar}

type ItemDef struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Slot     string      `json:"slot"`
	Rarity   Rarity      `json:"rarity"`
	Flavour  string      `json:"flavour"`
	Driver   DriverStats `json:"driver"`
	Power    float64     `json:"power,omitempty"`
	Grip     float64     `json:"grip,omitempty"`
	Handling float64     `json:"handling,omitempty"`
	Braking  float64     `json:"braking,omitempty"`
	WeightKg float64     `json:"weight_kg,omitempty"`
	// BaseValue is the cash reference price; K's Office marks it up.
	BaseValue int64 `json:"base_value"`
	// Tradeable items can appear in K's Office stock.
	Tradeable bool `json:"tradeable"`
}

var Items = []ItemDef{
	// --- common ---
	{ID: "cotton_gloves", Name: "Cotton Work Gloves", Slot: SlotHands, Rarity: Common, BaseValue: 120, Tradeable: true,
		Flavour: "Better than bare hands on a cold wheel.", Driver: DriverStats{Reflexes: 1}},
	{ID: "canvas_shoes", Name: "Thin-Sole Canvas Shoes", Slot: SlotFeet, Rarity: Common, BaseValue: 140, Tradeable: true,
		Flavour: "You can feel the pedal. That's the whole point.", Driver: DriverStats{Consistency: 1}},
	{ID: "shop_cap", Name: "Tuning Shop Cap", Slot: SlotHead, Rarity: Common, BaseValue: 110, Tradeable: true,
		Flavour: "Faded logo, permanent oil stain.", Driver: DriverStats{Intelligence: 1}},
	{ID: "coveralls", Name: "Grease-Stained Coveralls", Slot: SlotSuit, Rarity: Common, BaseValue: 150, Tradeable: true,
		Flavour: "Smells like two-stroke and instant coffee.", Driver: DriverStats{Stamina: 1}},
	{ID: "paper_map", Name: "Dog-Eared Pass Map", Slot: SlotMind, Rarity: Common, BaseValue: 130, Tradeable: true,
		Flavour: "Every corner numbered in blue biro.", Driver: DriverStats{Intelligence: 1}},

	// --- uncommon ---
	{ID: "driving_gloves", Name: "Perforated Driving Gloves", Slot: SlotHands, Rarity: Uncommon, BaseValue: 480, Tradeable: true,
		Flavour: "Grip without the sweat.", Driver: DriverStats{Reflexes: 2, Consistency: 1}},
	{ID: "driving_shoes", Name: "Narrow Driving Shoes", Slot: SlotFeet, Rarity: Uncommon, BaseValue: 520, Tradeable: true,
		Flavour: "Heel-toe stops being a coin flip.", Driver: DriverStats{Consistency: 2, Reflexes: 1}},
	{ID: "helmet_open", Name: "Open-Face Helmet", Slot: SlotHead, Rarity: Uncommon, BaseValue: 560, Tradeable: true,
		Flavour: "Cheap, loud, and it does the job.", Driver: DriverStats{Bravery: 2}},
	{ID: "strut_brace", Name: "Front Strut Brace", Slot: SlotCar, Rarity: Uncommon, BaseValue: 640, Tradeable: true,
		Flavour: "Turn-in gets a little less vague.", Handling: 3},
	{ID: "lucky_charm", Name: "Roadside Shrine Charm", Slot: SlotCharm, Rarity: Uncommon, BaseValue: 500, Tradeable: true,
		Flavour: "Bought at the vending machine at the top. Works, apparently.", Driver: DriverStats{Consistency: 2}},

	// --- rare ---
	{ID: "lsd_unit", Name: "Two-Way LSD", Slot: SlotCar, Rarity: Rare, BaseValue: 2600, Tradeable: true,
		Flavour: "Both wheels, all the time.", Grip: 5, Handling: 3},
	{ID: "bucket_seat", Name: "Full Bucket Seat", Slot: SlotCar, Rarity: Rare, BaseValue: 2400, Tradeable: true,
		Flavour: "You stop bracing with your knees and start driving.", Handling: 2, WeightKg: -12, Driver: DriverStats{Consistency: 2}},
	{ID: "nomex_suit", Name: "Nomex Racing Suit", Slot: SlotSuit, Rarity: Rare, BaseValue: 2200, Tradeable: true,
		Flavour: "Hot, heavy, and reassuring.", Driver: DriverStats{Stamina: 3, Bravery: 2}},
	{ID: "night_goggles", Name: "Amber Night Lenses", Slot: SlotHead, Rarity: Rare, BaseValue: 2100, Tradeable: true,
		Flavour: "The pass at 2am stops being a tunnel.", Driver: DriverStats{Reflexes: 3, Intelligence: 1}},
	{ID: "stopwatch", Name: "Worn Sector Stopwatch", Slot: SlotMind, Rarity: Rare, BaseValue: 2300, Tradeable: true,
		Flavour: "Split times going back six years, all in pencil.", Driver: DriverStats{Intelligence: 3, Consistency: 1}},

	// --- epic ---
	{ID: "titanium_shift", Name: "Titanium Short Shifter", Slot: SlotCar, Rarity: Epic, BaseValue: 9500, Tradeable: true,
		Flavour: "Half the throw, twice the confidence.", Handling: 4, Driver: DriverStats{Reflexes: 3}},
	{ID: "carbon_driveshaft", Name: "Carbon Driveshaft", Slot: SlotCar, Rarity: Epic, BaseValue: 11000, Tradeable: true,
		Flavour: "Spins up like there's nothing behind the gearbox.", Power: 12, WeightKg: -14},
	{ID: "impact_earrings", Name: "Blue Lightning Earrings", Slot: SlotCharm, Rarity: Epic, BaseValue: 10500, Tradeable: true,
		Flavour: "Two of them. They only ever race as a pair.", Driver: DriverStats{Reflexes: 4, Bravery: 2}},
	{ID: "team_jacket", Name: "Downhill Team Jacket", Slot: SlotSuit, Rarity: Epic, BaseValue: 9800, Tradeable: true,
		Flavour: "Project stitching on the back. You did not earn this.", Driver: DriverStats{Stamina: 3, Consistency: 3}},
	{ID: "sector_notebook", Name: "Rival Data Notebook", Slot: SlotMind, Rarity: Epic, BaseValue: 10200, Tradeable: true,
		Flavour: "Every regular on the mountain, timed and diagrammed.", Driver: DriverStats{Intelligence: 4, Consistency: 2}},

	// --- legendary: the named drops ---
	{ID: "keiichi_gloves", Name: "Keiichi's Gloves", Slot: SlotHands, Rarity: Legendary, BaseValue: 68000, Tradeable: true,
		Flavour: "Worn through at the palms from a decade of opposite lock.",
		Driver:  DriverStats{Reflexes: 6, Bravery: 3}},
	{ID: "takumi_brakes", Name: "Takumi's Brakes", Slot: SlotCar, Rarity: Legendary, BaseValue: 74000, Tradeable: true,
		Flavour: "Pads worn to the backing on one side only. He never uses the other.",
		Braking: 10, Driver: DriverStats{Consistency: 4}},
	{ID: "ryosuke_knowledge", Name: "Ryosuke's Knowledge", Slot: SlotMind, Rarity: Legendary, BaseValue: 82000, Tradeable: true,
		Flavour: "Not a book. A way of looking at a corner before you reach it.",
		Driver:  DriverStats{Intelligence: 8, Consistency: 3}},
	{ID: "bunta_cup", Name: "Bunta's Paper Cup", Slot: SlotCharm, Rarity: Legendary, BaseValue: 71000, Tradeable: true,
		Flavour: "Still full. Still level. Nobody knows how.",
		Driver:  DriverStats{Consistency: 6, Intelligence: 4}},
	{ID: "godfoot_sandals", Name: "Sandals of the God Foot", Slot: SlotFeet, Rarity: Legendary, BaseValue: 66000, Tradeable: true,
		Flavour: "Sandals. On the mountain. At night. And he still beat you.",
		Driver:  DriverStats{Bravery: 7, Reflexes: 2}},
	{ID: "godhand_helmet", Name: "God Hand Helmet", Slot: SlotHead, Rarity: Legendary, BaseValue: 70000, Tradeable: true,
		Flavour: "Visor scratched by forty seasons of gravel.",
		Driver:  DriverStats{Intelligence: 5, Reflexes: 4}},
	{ID: "emperor_suit", Name: "Emperor Team Overalls", Slot: SlotSuit, Rarity: Legendary, BaseValue: 64000, Tradeable: true,
		Flavour: "Discipline, stitched in.",
		Driver:  DriverStats{Stamina: 6, Bravery: 4}},
}

func ItemByID(id string) (ItemDef, bool) {
	for _, it := range Items {
		if it.ID == id {
			return it, true
		}
	}
	return ItemDef{}, false
}

// RarityWeights drive the drop table. Quality (0..1) from the job/race tier
// shifts probability mass toward the top end.
func RarityWeights(quality float64) map[Rarity]float64 {
	if quality < 0 {
		quality = 0
	}
	if quality > 1 {
		quality = 1
	}
	return map[Rarity]float64{
		Common:    60 - 45*quality,
		Uncommon:  28 - 8*quality,
		Rare:      10 + 14*quality,
		Epic:      1.8 + 14*quality,
		Legendary: 0.2 + 3.3*quality,
	}
}

// SellValue is what a duplicate fetches back. Deliberately well under the
// shop price so buying is never a money loop.
func SellValue(def ItemDef, quality float64) int64 {
	v := float64(def.BaseValue) * quality * 0.25
	if v < 1 {
		v = 1
	}
	return int64(v)
}

// ShopPrice is what K's Office charges. Legendaries carry a heavy markup
// because they are droppable — the shop is the impatient path, not the cheap one.
func ShopPrice(def ItemDef, quality float64) int64 {
	mult := 2.2
	switch def.Rarity {
	case Epic:
		mult = 3.0
	case Legendary:
		mult = 4.0
	}
	return int64(float64(def.BaseValue) * quality * mult)
}

// ---------------------------------------------------------------------------
// Liveries — cosmetic only, never touch a stat
// ---------------------------------------------------------------------------

type LiveryDef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
	PriceCash int64  `json:"price_cash"`
	PriceRuby int64  `json:"price_rubies"`
	Starter   bool   `json:"starter"`
}

var Liveries = []LiveryDef{
	{ID: "panda", Name: "Panda Two-Tone", Primary: "#F2F2F0", Secondary: "#16181C", PriceCash: 0, Starter: true},
	{ID: "midnight", Name: "Midnight Purple", Primary: "#3B2A5A", Secondary: "#1A1030", PriceCash: 4000},
	{ID: "gunmetal", Name: "Gunmetal Works", Primary: "#4A5157", Secondary: "#23272B", PriceCash: 6500},
	{ID: "sunrise", Name: "Sunrise Fade", Primary: "#E8622C", Secondary: "#F2B33D", PriceCash: 12000},
	{ID: "impact", Name: "Impact Blue", Primary: "#1E4FA8", Secondary: "#0B1E42", PriceRuby: 120},
	{ID: "kaido", Name: "Kaido Kanji Wrap", Primary: "#C4162B", Secondary: "#0F0F12", PriceRuby: 180},
	{ID: "ghost", Name: "Ghost Pearl", Primary: "#D8DDE4", Secondary: "#9AA6B2", PriceRuby: 260},
}

func LiveryByID(id string) (LiveryDef, bool) {
	for _, l := range Liveries {
		if l.ID == id {
			return l, true
		}
	}
	return LiveryDef{}, false
}

const StarterLivery = "panda"

// ---------------------------------------------------------------------------
// Ruby packs (real-money IAP)
// ---------------------------------------------------------------------------

type RubyPack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Rubies   int64  `json:"rubies"`
	Bonus    int64  `json:"bonus"`
	PriceUSD string `json:"price_usd"`
}

var RubyPacks = []RubyPack{
	{ID: "pack_handful", Name: "Handful of Rubies", Rubies: 100, PriceUSD: "0.99"},
	{ID: "pack_pouch", Name: "Pouch of Rubies", Rubies: 550, Bonus: 50, PriceUSD: "4.99"},
	{ID: "pack_case", Name: "Case of Rubies", Rubies: 1200, Bonus: 200, PriceUSD: "9.99"},
	{ID: "pack_crate", Name: "Crate of Rubies", Rubies: 2600, Bonus: 700, PriceUSD: "19.99"},
	{ID: "pack_pallet", Name: "Pallet of Rubies", Rubies: 7000, Bonus: 2500, PriceUSD: "49.99"},
}

func RubyPackByID(id string) (RubyPack, bool) {
	for _, p := range RubyPacks {
		if p.ID == id {
			return p, true
		}
	}
	return RubyPack{}, false
}
