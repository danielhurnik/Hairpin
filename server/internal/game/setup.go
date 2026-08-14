package game

import "math"

// ---------------------------------------------------------------------------
// Setup — what the spanner does that the wallet cannot
//
// A setup is not an upgrade. Upgrades add performance; a setup MOVES it, from
// the parts of a road you have decided not to care about onto the parts you
// have. WithSetup divides every sheet by what it is worth to that particular
// car on average, so no amount of fiddling raises an overall rating by a
// point. What tuning buys you is being right about the road in front of you.
//
// Every axis works on the geometry rather than on a corner's label. Ride
// height is millimetres measured against how far the road actually moves under
// the car; camber is degrees against how long the tyre is loaded; the final
// drive is a ratio against how much of the straight is spent near the limiter.
// That is what makes the numbers worth setting precisely, and it is why they
// disagree from corner to corner within a single pass.
//
// You can only adjust what you have bought the parts for. A stock car has
// nothing to turn: no coilovers, no adjustable arms, no boost controller.
// ---------------------------------------------------------------------------

const (
	SetupMin = -2
	SetupMax = 2
)

// setupGain scales the whole sheet at once. It is the single knob that sets
// how much a perfect setup is worth against a factory one — the design target
// is about 5%, i.e. roughly a ladder rung, which
// TestPerfectSetupIsWorthAboutFivePercent holds to.
const setupGain = 2.05

type Setup struct {
	Gearing  int `json:"gearing"`
	Boost    int `json:"boost"`
	Pressure int `json:"pressure"`
	Ride     int `json:"ride_height"`
	Camber   int `json:"camber"`
	Damping  int `json:"damping"`
	Ballast  int `json:"ballast"`
}

// SetupAxis describes one adjustment: what unlocks it, what it is measured in,
// and what the ends of the range mean. Values holds the real setting at each
// notch from SetupMin to SetupMax, so the garage shows 85mm rather than "-1".
type SetupAxis struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	MinTier  int    `json:"min_tier"`
	// Part names the thing you bolted on, so a locked axis can explain itself
	// in the language of the car rather than of the schema.
	Part   string     `json:"part"`
	Unit   string     `json:"unit"`
	Values [5]float64 `json:"values"`
	// Decimals is how precisely to print the value.
	Decimals int    `json:"decimals"`
	Low      string `json:"low"`
	High     string `json:"high"`
	Note     string `json:"note"`
}

// SetupAxes is the full sheet. Suspension carries three of the seven, because
// suspension is where setup work actually lives.
//
// Values run from SetupMin to SetupMax in order, so index 2 is the factory
// setting the car left the workshop on.
var SetupAxes = []SetupAxis{
	{
		ID: "gearing", Label: "Final Drive", Category: CatEngine, MinTier: 1,
		Part: "Intake & Exhaust", Unit: "", Decimals: 2,
		Values: [5]float64{3.70, 3.90, 4.10, 4.30, 4.50},
		Low:    "Long", High: "Short",
		Note: "Short gears fire you out of slow corners and run out of road on a long straight.",
	},
	{
		ID: "boost", Label: "Boost Pressure", Category: CatTurbo, MinTier: 1,
		Part: "Bolt-On Snail", Unit: "bar", Decimals: 1,
		Values: [5]float64{0.4, 0.6, 0.8, 1.0, 1.2},
		Low:    "Low", High: "High",
		Note: "More boost is more everywhere it can reach the ground, and a longer wait for it out of a hairpin.",
	},
	{
		ID: "pressure", Label: "Tyre Pressure", Category: CatTires, MinTier: 1,
		Part: "Sport Touring", Unit: "bar", Decimals: 1,
		Values: [5]float64{1.8, 2.0, 2.2, 2.4, 2.6},
		Low:    "Low", High: "High",
		Note: "Low pressure buys grip now and hands it back once the tyres are hot.",
	},
	{
		ID: "ballast", Label: "Weight Balance", Category: CatWeight, MinTier: 1,
		Part: "Stripped Boot", Unit: "% front", Decimals: 0,
		Values: [5]float64{50, 52, 54, 56, 58},
		Low:    "Rearward", High: "Forward",
		Note: "Mass forward stops and turns in; mass back puts the power down. Your layout has opinions.",
	},
	{
		ID: "ride_height", Label: "Ride Height", Category: CatSuspension, MinTier: 2,
		Part: "Adjustable Coilovers", Unit: "mm", Decimals: 0,
		Values: [5]float64{130, 115, 100, 85, 70},
		Low:    "High", High: "Low",
		Note: "Low is quick where the road is smooth and fast, and grounds out where it is neither.",
	},
	{
		ID: "camber", Label: "Camber", Category: CatSuspension, MinTier: 3,
		Part: "Pillow-Ball Arms", Unit: "°", Decimals: 1,
		Values: [5]float64{-0.5, -1.2, -2.0, -2.8, -3.6},
		Low:    "Conservative", High: "Aggressive",
		Note: "Lean the wheels in for grip through a long corner. The braking zones will notice.",
	},
	{
		ID: "damping", Label: "Damping", Category: CatSuspension, MinTier: 4,
		Part: "2-Way Dampers", Unit: "clicks", Decimals: 0,
		Values: [5]float64{2, 5, 8, 11, 14},
		Low:    "Soft", High: "Stiff",
		Note: "Stiff changes direction the instant you ask, and finds every bump you hoped to ignore.",
	},
}

func (s Setup) Get(axis string) (int, bool) {
	switch axis {
	case "gearing":
		return s.Gearing, true
	case "boost":
		return s.Boost, true
	case "pressure":
		return s.Pressure, true
	case "ballast":
		return s.Ballast, true
	case "ride_height":
		return s.Ride, true
	case "camber":
		return s.Camber, true
	case "damping":
		return s.Damping, true
	}
	return 0, false
}

func (s Setup) With(axis string, v int) (Setup, bool) {
	switch axis {
	case "gearing":
		s.Gearing = v
	case "boost":
		s.Boost = v
	case "pressure":
		s.Pressure = v
	case "ballast":
		s.Ballast = v
	case "ride_height":
		s.Ride = v
	case "camber":
		s.Camber = v
	case "damping":
		s.Damping = v
	default:
		return s, false
	}
	return s, true
}

func AxisByID(id string) (SetupAxis, bool) {
	for _, a := range SetupAxes {
		if a.ID == id {
			return a, true
		}
	}
	return SetupAxis{}, false
}

// Setting is the real value at a notch — 85mm, 2.4 bar, 4.30.
func (a SetupAxis) Setting(notch int) float64 {
	i := notch - SetupMin
	if i < 0 {
		i = 0
	}
	if i >= len(a.Values) {
		i = len(a.Values) - 1
	}
	return a.Values[i]
}

// AxisUnlocked reports whether a car with these tiers has the part fitted.
func AxisUnlocked(a SetupAxis, tiers UpgradeTiers) bool {
	tier, ok := tiers.Get(a.Category)
	return ok && tier >= a.MinTier
}

// LegalSetup strips any axis the car has no part for, and pulls anything out
// of range back into it. Applied on the way in and again on the way out, so a
// tier that no longer covers an axis cannot leave a live adjustment behind.
func LegalSetup(s Setup, tiers UpgradeTiers) Setup {
	out := Setup{}
	for _, a := range SetupAxes {
		if !AxisUnlocked(a, tiers) {
			continue
		}
		v, _ := s.Get(a.ID)
		if v < SetupMin {
			v = SetupMin
		}
		if v > SetupMax {
			v = SetupMax
		}
		out, _ = out.With(a.ID, v)
	}
	return out
}

// ---------------------------------------------------------------------------
// What the road is asking for
//
// These read a piece of road and answer one question each, in 0..1. They are
// what the setup axes actually trade against, and they are why no setting is
// right twice: a corner that wants camber does not want the same ride height
// as the crest before it.
// ---------------------------------------------------------------------------

// loaded is how hard the tyres are working laterally, 0 on a straight.
func loaded(seg Segment) float64 {
	return clamp(seg.LateralG/0.95, 0, 1.2)
}

// sustained separates a long committed corner from a flick. Camber is worth
// something in the first and very little in the second.
func sustained(seg Segment) float64 {
	return loaded(seg) * clamp(seg.BaseSeconds/5.5, 0.25, 1.2)
}

// openRoad is how much of this segment is spent going fast in a straight line.
func openRoad(seg Segment) float64 {
	if seg.Radius > 0 {
		return 0.15 * clamp((seg.EntrySpeed-30)/25, 0, 1)
	}
	return clamp((math.Max(seg.EntrySpeed, seg.ExitSpeed)-26)/24, 0, 1)
}

// nearLimiter is how much of a straight is spent at the top of the gearing,
// which is the only place a long final drive pays.
func nearLimiter(seg Segment) float64 {
	if seg.Radius > 0 {
		return 0
	}
	return clamp((math.Max(seg.EntrySpeed, seg.ExitSpeed)-36)/18, 0, 1) *
		clamp(seg.Length/320, 0.3, 1)
}

// crawling is how slow the exit is — where short gearing, boost lag and
// traction all decide themselves.
func crawling(seg Segment) float64 {
	if seg.Radius <= 0 {
		return 0
	}
	return clamp((23-seg.ExitSpeed)/15, 0, 1)
}

// stopping is how much speed this piece of road takes away, plus what gravity
// is doing to the brakes.
func stopping(seg Segment) float64 {
	var s float64
	if seg.EntrySpeed > 1 && seg.ExitSpeed < seg.EntrySpeed {
		s = clamp((seg.EntrySpeed-seg.ExitSpeed)/seg.EntrySpeed/0.42, 0, 1)
	}
	return clamp(s+0.45*clamp(-seg.Gradient/10, 0, 1), 0, 1.2)
}

// transitions is direction-change work: a rhythm section asks the car to move
// from one side to the other and back without arguing.
func transitions(seg Segment) float64 {
	if seg.Rhythm {
		return 1
	}
	if seg.Radius > 0 {
		return 0.2
	}
	return 0
}

// rough is how much the road moves under the car, normalised against the
// smoothest surface in the game, with a crest counting for extra.
func rough(seg Segment) float64 {
	r := clamp((seg.Surface.bumpAmplitude()-18)/67, 0, 1)
	if seg.Crest {
		r = clamp(r+0.35, 0, 1.2)
	}
	return r
}

// heat grows through a run and only where the tyres are being worked.
func heat(seg Segment, progress float64) float64 {
	return clamp((progress-0.28)/0.72, 0, 1) * loaded(seg)
}

// ---------------------------------------------------------------------------
// The axes, in physical terms
// ---------------------------------------------------------------------------

// setupModRaw is what a sheet does to one piece of road, before WithSetup
// divides out whatever it was worth on average to this car.
func setupModRaw(b Build, seg Segment, progress float64) float64 {
	s := b.Setup
	if s == (Setup{}) {
		return 1
	}
	mod := 1.0

	// Everything below is scaled by what the car actually is, not only by what
	// the road is. This is the difference between a sheet you can publish and
	// a sheet you have to work out: a 300hp car does not want the gearing a
	// 130hp car wants, and a big laggy turbo cares about boost in a way a
	// small one does not.
	power := b.Car.PowerToWt / ReferencePowerToWt
	grip := b.Car.Grip / ReferenceGrip
	heavy := clamp(b.Car.Weight/1100, 0.75, 1.5)
	// How little grip there is to go round tonight. Zero on the dry reference
	// road, which is what keeps ratings and calibration quoted against one
	// fixed condition while an actual race is quoted against the weather.
	slick := clamp(seg.Wet*0.85+seg.Cold*0.6, 0, 1)

	// --- Final drive: exit versus terminal speed -------------------------
	if s.Gearing != 0 {
		short := float64(s.Gearing) / 2 // +1 shortest, -1 longest
		// A car with power to spare pulls long gears happily and gains little
		// from short ones; an underpowered car lives on the short end.
		pull := clamp(1.45-0.55*power, 0.35, 1.45)
		reach := clamp(0.55+0.55*power, 0.55, 1.75)
		mod += setupGain * (0.034*short*crawling(seg)*pull - 0.030*short*nearLimiter(seg)*reach)
	}

	// --- Boost: power where it lands, lag where it does not ---------------
	if s.Boost != 0 {
		bo := float64(s.Boost) / 2
		// A big single is all boost and all lag; a small bolt-on barely
		// notices the controller either way.
		snail := clamp(b.Car.TurboLag/0.012, 0.25, 1.6)
		// On a greasy road a big lump of boost arriving mid-corner is a
		// liability rather than a lap time.
		mod += setupGain * snail * (0.034*bo*openRoad(seg) -
			0.028*bo*crawling(seg)*(1+1.1*slick))
	}

	// --- Tyre pressure: grip now against grip later -----------------------
	if s.Pressure != 0 {
		soft := -float64(s.Pressure) / 2 // low pressure is the soft end
		// Soft is worth far more when there is no grip to begin with.
		mod += setupGain * (0.030*soft*loaded(seg)*(1+0.75*slick) - 0.026*soft*openRoad(seg))
		if soft > 0 {
			// Heavy cars cook their tyres, and so do cars generating a lot of
			// grip to begin with — but nothing overheats on a wet cold road.
			mod -= setupGain * 0.030 * soft * heat(seg, progress) * heavy *
				clamp(grip, 0.7, 1.5) * (1 - 0.8*slick)
		}
	}

	// --- Weight balance: stopping against driving out ---------------------
	//
	// This is the one axis where the layout changes the answer rather than
	// just its size. Weight over the driven wheels is traction, so moving mass
	// forward helps a front-driven car off a slow corner and hurts everything
	// else — a scale factor could never express that, and while ballast was
	// only scaled, every layout wanted the identical sheet.
	if s.Ballast != 0 {
		fwd := float64(s.Ballast) / 2
		mod += setupGain * ballastLayout(b.Car.Drivetrain) *
			(0.032*fwd*stopping(seg) - 0.030*fwd*crawling(seg)*tractionAxle(b.Car.Drivetrain))
	}

	// --- Ride height: millimetres against how far the road moves ----------
	if s.Ride != 0 {
		axis, _ := AxisByID("ride_height")
		h := axis.Setting(s.Ride)
		low := (100 - h) / 30 // +1 at 70mm, -1 at 130mm
		// Slamming the car pays on a dry road and asks for trouble on a wet
		// one, where the travel is what lets the tyre follow the surface.
		mod += setupGain * 0.030 * low * loaded(seg) * (1 - 0.9*slick)
		// Clearance is the real number: how much room is left once the road
		// has finished moving. A heavy car uses more travel getting there, so
		// it runs out sooner than a light one at the same ride height.
		if clear := h - seg.Surface.bumpAmplitude()*groundingFactor(seg)*heavy; clear < 30 {
			mod -= setupGain * 0.055 * clamp((30-clear)/30, 0, 1) * rough(seg)
		}
	}

	// --- Camber: sustained load against everything longitudinal -----------
	if s.Camber != 0 {
		agg := float64(s.Camber) / 2
		// Diminishing returns: a car already on a lot of grip has less to
		// find by leaning the wheels over, and more to lose under braking.
		// Diminishing returns with grip, and in the wet the tyre wants to be
		// flat on the road rather than leaning on an edge.
		gain := clamp(1.35-0.4*grip, 0.55, 1.35) * (1 - 0.85*slick)
		mod += setupGain * (0.040*agg*sustained(seg)*gain -
			0.036*agg*(stopping(seg)*0.6+openRoad(seg)*0.4)*(1+0.5*slick))
	}

	// --- Damping: direction change against compliance ---------------------
	if s.Damping != 0 {
		stiff := float64(s.Damping) / 2
		// Stiff dampers work with a car that has the grip to use them and
		// punish a heavy one that needs the travel.
		mod += setupGain * (0.034*stiff*transitions(seg)*clamp(grip, 0.7, 1.4) -
			0.032*stiff*rough(seg)*heavy)
	}

	// A setup can shade a corner, never transform it.
	return clamp(mod, 0.82, 1.18)
}

// groundingFactor scales the bump the car actually has to clear. A crest
// compresses the suspension on top of whatever the surface is doing.
func groundingFactor(seg Segment) float64 {
	f := 1.0
	if seg.Crest {
		f += 0.6
	}
	// Loading the car in a corner uses travel that is then not available.
	return f + 0.35*loaded(seg)
}

// ballastLayout scales weight transfer by where the engine already is.
func ballastLayout(dt Drivetrain) float64 {
	switch dt {
	case FF:
		return 0.7 // already nose-heavy; there is less to gain
	case MR:
		return 1.35 // the most sensitive thing here to move mass in
	case AWD:
		return 0.9
	}
	return 1.0 // FR
}

// tractionAxle is how much moving mass forward costs traction out of a slow
// corner. Negative for front-wheel drive, where the driven wheels are the ones
// you just put the weight on.
func tractionAxle(dt Drivetrain) float64 {
	switch dt {
	case FF:
		return -0.8
	case MR:
		return 1.3
	case AWD:
		return 0.4
	}
	return 1.0 // FR
}

// ---------------------------------------------------------------------------
// Keeping it a trade
// ---------------------------------------------------------------------------

// setupNormFor is what keeps "a setup cannot make you faster overall" true for
// every car rather than only for an average one.
//
// The effects above are honest physics and make no attempt to cancel out. A
// rating is a performance-weighted average over every road in the game, so
// dividing by the build's own weighted average of the modifier leaves the
// overall figure exactly where it was and lets the per-track figure move. It
// costs one pass over the six passes per build, once per race rather than once
// per corner.
func setupNormFor(car CarStats, driver DriverStats, s Setup) float64 {
	if s == (Setup{}) {
		return 1
	}
	bare := Build{Car: car, Driver: driver}
	withSetup := Build{Car: car, Driver: driver, Setup: s, SetupNorm: 1}

	// This has to mirror OverallRating exactly to cancel exactly: per-track
	// weighted means first, then a flat average across the six. Weighting the
	// whole game by time instead — which a first pass did — quietly favours
	// whichever setup suits the longer passes, and the passes are no longer
	// the same length now that their duration comes out of their geometry.
	var num, den float64
	for _, t := range Tracks {
		n := len(t.Segments)
		var tNum, tDen, tBase float64
		for i, seg := range t.Segments {
			progress := float64(i) / math.Max(1, float64(n-1))
			p := segmentPerf(bare, seg, progress) * seg.BaseSeconds
			tNum += p * setupModRaw(withSetup, seg, progress)
			tDen += p
			tBase += seg.BaseSeconds
		}
		if tBase == 0 {
			continue
		}
		num += tNum / tBase
		den += tDen / tBase
	}
	if den == 0 || num == 0 {
		return 1
	}
	return num / den
}

// WithSetup attaches a sheet to a build and works out the constant that keeps
// it honest. Every build that races or gets rated goes through here.
func WithSetup(b Build, s Setup) Build {
	b.Setup = s
	b.SetupNorm = setupNormFor(b.Car, b.Driver, s)
	return b
}

// setupMod is the honest modifier: the trade, divided by whatever that trade
// was worth on average to this particular car.
func setupMod(b Build, seg Segment, progress float64) float64 {
	raw := setupModRaw(b, seg, progress)
	if b.SetupNorm <= 0 {
		return raw
	}
	return raw / b.SetupNorm
}

// ---------------------------------------------------------------------------
// Risk
// ---------------------------------------------------------------------------

// SetupVariance is what a setup does to how repeatable the car is. Unlike the
// performance side this is deliberately NOT normalised away: leaning on the
// car makes it faster on its day and messier on the others.
//
// The two knobs are not the same kind of thing, and the difference is what
// makes aggression a strategy rather than a tax. Spread is symmetric and worth
// having when you are the slower car; a mistake is time lost and never found.
// So aggression moves spread hard and mistakes gently.
type SetupVariance struct {
	SigmaMult   float64
	MistakeMult float64
}

func setupVariance(s Setup) SetupVariance {
	v := SetupVariance{SigmaMult: 1, MistakeMult: 1}

	v.SigmaMult += float64(s.Boost) * 0.100
	v.MistakeMult += float64(s.Boost) * 0.015

	v.SigmaMult += float64(s.Camber) * 0.030
	v.MistakeMult += float64(s.Camber) * 0.030

	v.SigmaMult += float64(s.Damping) * 0.060
	v.MistakeMult += float64(s.Damping) * 0.010

	v.SigmaMult -= float64(s.Pressure) * 0.025

	// A car on the floor has no travel left to absorb a mistake.
	v.MistakeMult += float64(s.Ride) * 0.020

	v.SigmaMult = clamp(v.SigmaMult, 0.75, 1.45)
	v.MistakeMult = clamp(v.MistakeMult, 0.80, 1.40)
	return v
}

// ---------------------------------------------------------------------------
// Binning it
// ---------------------------------------------------------------------------

const (
	// CrashBase is the chance of losing a car per corner, before anything
	// about the road, the sheet or the driver is taken into account. It is
	// deliberately tiny: a factory car on a dry pass should essentially never
	// crash, and a wild sheet on a wet one should be a real gamble.
	CrashBase = 0.0016
	// CrashAggressionCap stops the most extreme sheet ever becoming a
	// certainty on a single corner.
	CrashAggressionCap = 4.5
)

// crashAggression is how far a sheet is asking to be caught out.
//
// These are the settings that take away the car's margin rather than trading
// one part of the road for another: no ride height to absorb a bump, no travel
// left in the dampers, an edge of a tyre instead of the flat of it, and a lump
// of boost arriving whenever it feels like it. None of them are wrong — they
// are all quick somewhere — but a sheet with all of them on it is one bad
// piece of road away from a repair bill.
func crashAggression(s Setup) float64 {
	a := 1.0
	a += 0.42 * clamp(float64(s.Ride)/2, 0, 1)      // slammed
	a += 0.34 * clamp(float64(s.Damping)/2, 0, 1)   // no compliance
	a += 0.30 * clamp(float64(s.Camber)/2, 0, 1)    // on the edge of the tyre
	a += 0.26 * clamp(float64(s.Boost)/2, 0, 1)     // sudden delivery
	a += 0.18 * clamp(-float64(s.Pressure)/2, 0, 1) // soft and squirming
	return clamp(a, 1, CrashAggressionCap)
}

// crashChance is the probability of throwing it away in one corner.
//
// Everything that makes a corner frightening is already in seg.Risk — no room,
// no camber, no view, a broken surface, and a wet night — so this is that
// number against how much margin the sheet left and how much the driver has in
// hand. A calm sheet and a consistent driver bring it close to nothing.
func crashChance(b Build, seg Segment) float64 {
	// Only the genuinely difficult corners can catch you out.
	bite := clamp((seg.Risk-0.35)/0.6, 0, 1)
	if bite == 0 {
		return 0
	}
	agg := crashAggression(b.Setup)

	// Consistency is the stat that keeps it on the road; bravery is what puts
	// you in a position to leave it in the first place.
	steady := clamp(ReferenceDriverStat/math.Max(b.Driver.Consistency, 1), 0.45, 1.9)
	brave := clamp(1+(b.Driver.Bravery-ReferenceDriverStat)/ReferenceDriverStat*0.30, 0.7, 1.7)

	p := CrashBase * bite * (agg - 1 + 0.35) * steady * brave
	return clamp(p, 0, 0.08)
}

// RepairBill is what putting the car straight costs after a crash: a
// proportion of what has been spent on it, so the more there is to break the
// more there is to fix. It is what makes an aggressive sheet a decision rather
// than a free roll of the dice.
func RepairBill(chassisID string, tiers UpgradeTiers) int64 {
	value := int64(0)
	if ch, ok := ChassisByID(chassisID); ok {
		value += ch.Price
	}
	for _, cat := range UpgradeCategories {
		tier, _ := tiers.Get(cat)
		for i := 1; i <= tier; i++ {
			if c, ok := UpgradeCost(cat, i); ok {
				value += c
			}
		}
	}
	bill := value / 22
	if bill < 250 {
		bill = 250
	}
	if bill > 40000 {
		bill = 40000
	}
	return bill
}

// ---------------------------------------------------------------------------
// What the run told you
// ---------------------------------------------------------------------------

// SetupNote is one thing the car did, and where it did it. This is the only
// feedback the game gives about a sheet, and it arrives after the race rather
// than before it: the player is told what happened, never what to set.
type SetupNote struct {
	Segment string `json:"segment"`
	Text    string `json:"text"`
	// Cost is how much time the sheet gave away there, as a fraction. It is
	// used to rank the notes and is not published.
	Cost float64 `json:"-"`
}

// SetupFeedback reads back over the run and reports where the sheet hurt.
//
// Each note comes from the same term in the model that did the damage, so it
// cannot flatter or mislead: if it says the car was grounding out, the ride
// height genuinely cost time on that corner. What it deliberately never says
// is which way to turn the spanner.
func SetupFeedback(b Build, t TrackDef) []SetupNote {
	s := b.Setup
	if s == (Setup{}) {
		return nil
	}
	var notes []SetupNote
	n := len(t.Segments)
	heavy := clamp(b.Car.Weight/1100, 0.75, 1.5)

	for i, seg := range t.Segments {
		progress := float64(i) / math.Max(1, float64(n-1))
		slick := clamp(seg.Wet*0.85+seg.Cold*0.6, 0, 1)

		if s.Ride > 0 {
			axis, _ := AxisByID("ride_height")
			h := axis.Setting(s.Ride)
			if clear := h - seg.Surface.bumpAmplitude()*groundingFactor(seg)*heavy; clear < 30 {
				cost := 0.055 * clamp((30-clear)/30, 0, 1) * rough(seg)
				notes = append(notes, SetupNote{seg.Name,
					"The floor found the road through " + seg.Name + ".", cost})
			}
		}
		if s.Damping > 0 && rough(seg) > 0.5 {
			notes = append(notes, SetupNote{seg.Name,
				"The car skated over the broken surface in " + seg.Name + " instead of settling.",
				0.032 * float64(s.Damping) / 2 * rough(seg) * heavy})
		}
		if s.Camber > 0 && stopping(seg) > 0.55 {
			notes = append(notes, SetupNote{seg.Name,
				"Not enough tyre on the road under braking for " + seg.Name + ".",
				0.036 * float64(s.Camber) / 2 * stopping(seg) * (1 + 0.5*slick)})
		}
		if s.Boost > 0 && crawling(seg) > 0.5 {
			notes = append(notes, SetupNote{seg.Name,
				"You waited for the turbo on the way out of " + seg.Name + ".",
				0.028 * float64(s.Boost) / 2 * crawling(seg) * (1 + 1.1*slick)})
		}
		if s.Pressure < 0 && heat(seg, progress) > 0.45 {
			notes = append(notes, SetupNote{seg.Name,
				"The tyres were long gone by " + seg.Name + ".",
				0.030 * -float64(s.Pressure) / 2 * heat(seg, progress) * heavy * (1 - 0.8*slick)})
		}
		if s.Gearing > 0 && nearLimiter(seg) > 0.5 {
			notes = append(notes, SetupNote{seg.Name,
				"Bouncing off the limiter down " + seg.Name + ".",
				0.030 * float64(s.Gearing) / 2 * nearLimiter(seg)})
		}
		if s.Gearing < 0 && crawling(seg) > 0.55 {
			notes = append(notes, SetupNote{seg.Name,
				"Nothing there off the corner in " + seg.Name + ".",
				0.034 * -float64(s.Gearing) / 2 * crawling(seg)})
		}
	}

	// Only the worst few are worth saying. A list of every compromise is a
	// spreadsheet, and a spreadsheet is the thing this is trying not to be.
	for i := 1; i < len(notes); i++ {
		for j := i; j > 0 && notes[j].Cost > notes[j-1].Cost; j-- {
			notes[j], notes[j-1] = notes[j-1], notes[j]
		}
	}
	if len(notes) > 3 {
		notes = notes[:3]
	}
	return notes
}
