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

// SetupMin and SetupMax are the ends of every axis, expressed as a normalised
// position rather than as a unit: -2 is one end of what the workshop can
// reach, +2 the other, 0 whatever the car left the factory on.
//
// The position is an internal coordinate and never leaves the server on its
// own. Everything a player reads or types is the real number — 87mm, 2.35
// bar, 4.22 — and SetupAxis.Values below is the calibration between the two.
// Keeping the model on a normalised axis is what lets one line of physics
// serve seven adjustments measured in five different units; keeping the
// player on the real number is what makes precision worth having.
const (
	SetupMin = -2.0
	SetupMax = 2.0
)

// setupGain scales the whole sheet at once. It is the single knob that sets
// how much a perfect setup is worth against a factory one — the design target
// is about 5%, i.e. roughly a ladder rung, which
// TestPerfectSetupIsWorthAboutFivePercent holds to.
const setupGain = 2.25

// Setup is one car's sheet, as positions on the axes above.
type Setup struct {
	Gearing  float64 `json:"gearing"`
	Boost    float64 `json:"boost"`
	Pressure float64 `json:"pressure"`
	Ride     float64 `json:"ride_height"`
	Camber   float64 `json:"camber"`
	Damping  float64 `json:"damping"`
	Ballast  float64 `json:"ballast"`
}

// IsFactory reports whether this is the car as it left the workshop.
//
// A tolerance rather than a comparison against the zero value, because a
// player who winds an axis back to exactly the factory number arrives there
// through two floating-point conversions and lands a few parts in a
// quadrillion off it. That is the factory setting by any measure that matters.
func (s Setup) IsFactory() bool {
	for _, a := range SetupAxes {
		if v, _ := s.Get(a.ID); math.Abs(v) > 1e-9 {
			return false
		}
	}
	return true
}

// SetupAxis describes one adjustment: what unlocks it, what it is measured in,
// and what the ends of the range mean.
//
// Values is the calibration curve — the real setting at each whole position
// from SetupMin to SetupMax — and everything between them is interpolated, so
// the axis is continuous rather than a choice of five. Step is the granularity
// the workshop actually works to: a millimetre of ride height, a tenth of a
// degree of camber, one click of damping. Between them they mean the player
// sets 87mm and the model is handed a position.
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
	// Step is the smallest adjustment that can be made, in Unit. Nothing here
	// is infinitely divisible: a coilover has a thread pitch and a damper has
	// clicks.
	Step float64 `json:"step"`
	// Decimals is how precisely to print the value. Fine enough to show Step.
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
		Part: "Intake & Exhaust", Unit: "", Step: 0.01, Decimals: 2,
		Values: [5]float64{3.70, 3.90, 4.10, 4.30, 4.50},
		Low:    "Long", High: "Short",
		Note: "Short gears fire you out of slow corners and run out of road on a long straight.",
	},
	{
		ID: "boost", Label: "Boost Pressure", Category: CatTurbo, MinTier: 1,
		Part: "Bolt-On Snail", Unit: "bar", Step: 0.05, Decimals: 2,
		Values: [5]float64{0.4, 0.6, 0.8, 1.0, 1.2},
		Low:    "Low", High: "High",
		Note: "More boost is more everywhere it can reach the ground, and a longer wait for it out of a hairpin.",
	},
	{
		ID: "pressure", Label: "Tyre Pressure", Category: CatTires, MinTier: 1,
		Part: "Sport Touring", Unit: "bar", Step: 0.05, Decimals: 2,
		Values: [5]float64{1.8, 2.0, 2.2, 2.4, 2.6},
		Low:    "Low", High: "High",
		Note: "Low pressure buys grip now and hands it back once the tyres are hot.",
	},
	{
		ID: "ballast", Label: "Weight Balance", Category: CatWeight, MinTier: 1,
		Part: "Stripped Boot", Unit: "% front", Step: 0.5, Decimals: 1,
		Values: [5]float64{50, 52, 54, 56, 58},
		Low:    "Rearward", High: "Forward",
		Note: "Mass forward stops and turns in; mass back puts the power down. Your layout has opinions.",
	},
	{
		ID: "ride_height", Label: "Ride Height", Category: CatSuspension, MinTier: 2,
		Part: "Adjustable Coilovers", Unit: "mm", Step: 1, Decimals: 0,
		Values: [5]float64{130, 115, 100, 85, 70},
		Low:    "High", High: "Low",
		Note: "Low is quick where the road is smooth and fast, and grounds out where it is neither.",
	},
	{
		ID: "camber", Label: "Camber", Category: CatSuspension, MinTier: 3,
		Part: "Pillow-Ball Arms", Unit: "°", Step: 0.1, Decimals: 1,
		Values: [5]float64{-0.5, -1.2, -2.0, -2.8, -3.6},
		Low:    "Conservative", High: "Aggressive",
		Note: "Lean the wheels in for grip through a long corner. The braking zones will notice.",
	},
	{
		ID: "damping", Label: "Damping", Category: CatSuspension, MinTier: 4,
		Part: "2-Way Dampers", Unit: "clicks", Step: 1, Decimals: 0,
		Values: [5]float64{2, 5, 8, 11, 14},
		Low:    "Soft", High: "Stiff",
		Note: "Stiff changes direction the instant you ask, and finds every bump you hoped to ignore.",
	},
}

func (s Setup) Get(axis string) (float64, bool) {
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

func (s Setup) With(axis string, v float64) (Setup, bool) {
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

// Setting is the real value at a position — 87mm, 2.35 bar, 4.22 — read off
// the calibration curve and interpolated between its points.
func (a SetupAxis) Setting(pos float64) float64 {
	x := clamp(pos, SetupMin, SetupMax) - SetupMin
	i := int(math.Floor(x))
	if i >= len(a.Values)-1 {
		return a.Values[len(a.Values)-1]
	}
	return a.Values[i] + (a.Values[i+1]-a.Values[i])*(x-float64(i))
}

// SettingLow and SettingHigh are the ends of the range in real units, which is
// what a player is choosing between. Either may be the larger number: winding
// a car down is fewer millimetres and more degrees.
func (a SetupAxis) SettingLow() float64  { return a.Values[0] }
func (a SetupAxis) SettingHigh() float64 { return a.Values[len(a.Values)-1] }

// Factory is the setting the car left the workshop on.
func (a SetupAxis) Factory() float64 { return a.Values[(len(a.Values)-1)/2] }

// InRange reports whether a real setting is one this axis can actually reach.
// The curve is monotonic but can run either way, so the ends are sorted.
func (a SetupAxis) InRange(setting float64) bool {
	lo, hi := math.Min(a.SettingLow(), a.SettingHigh()), math.Max(a.SettingLow(), a.SettingHigh())
	// A step of slack at each end: a setting that quantises back inside the
	// range is one the workshop can reach, and rejecting 70.4mm when 70mm is
	// legal would be pedantry rather than validation.
	return setting >= lo-a.Step/2 && setting <= hi+a.Step/2
}

// PositionFor is the inverse of Setting: what the model should be handed when
// a player asks for 87mm. Out-of-range settings clamp to the end of the axis.
func (a SetupAxis) PositionFor(setting float64) float64 {
	for i := 0; i+1 < len(a.Values); i++ {
		v0, v1 := a.Values[i], a.Values[i+1]
		if v0 == v1 {
			continue
		}
		if setting >= math.Min(v0, v1) && setting <= math.Max(v0, v1) {
			return SetupMin + float64(i) + (setting-v0)/(v1-v0)
		}
	}
	// Off the end of the curve: whichever end it is off.
	if a.SettingLow() < a.SettingHigh() {
		if setting < a.SettingLow() {
			return SetupMin
		}
		return SetupMax
	}
	if setting > a.SettingLow() {
		return SetupMin
	}
	return SetupMax
}

// Quantize snaps a position to something the workshop could actually set:
// whole steps away from the factory setting, and inside the range.
//
// Counting from the factory rather than from the bottom of the range is what
// keeps the untouched car exactly untouched — the alternative lands a
// nominally factory sheet a rounding error away from zero, and then everything
// that asks "has this been tuned at all" has to know about floating point.
func (a SetupAxis) Quantize(pos float64) float64 {
	pos = clamp(pos, SetupMin, SetupMax)
	if a.Step <= 0 {
		return pos
	}
	v := a.Factory() + math.Round((a.Setting(pos)-a.Factory())/a.Step)*a.Step
	return clamp(a.PositionFor(v), SetupMin, SetupMax)
}

// AxisUnlocked reports whether a car with these tiers has the part fitted.
func AxisUnlocked(a SetupAxis, tiers UpgradeTiers) bool {
	tier, ok := tiers.Get(a.Category)
	return ok && tier >= a.MinTier
}

// LegalSetup strips any axis the car has no part for, pulls anything out of
// range back into it, and snaps what is left to a setting the workshop could
// have reached. Applied on the way in and again on the way out, so a tier that
// no longer covers an axis cannot leave a live adjustment behind, and no
// stored value can describe a car that could not exist.
func LegalSetup(s Setup, tiers UpgradeTiers) Setup {
	out := Setup{}
	for _, a := range SetupAxes {
		if !AxisUnlocked(a, tiers) {
			continue
		}
		v, _ := s.Get(a.ID)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		out, _ = out.With(a.ID, a.Quantize(v))
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

// stopping is how much of this piece of road is spent hauling the car down,
// with gravity's thumb on the scale.
//
// Gradient multiplies the braking rather than being braking. An earlier
// version added a flat term for any downhill, which made a 380-metre descent
// where the car is accelerating the whole way read as a braking zone — and
// since a steep pass is mostly that, every descent came out braking-dominated
// and every layout wanted its mass forward. Nothing was on the brakes; the
// road was merely pointing down.
//
// What is true is that a corner on a steep descent is on the brakes even when
// it sheds no speed, because holding a speed downhill is braking too. That
// part is kept, and only for corners.
func stopping(seg Segment) float64 {
	var s float64
	if seg.EntrySpeed > 1 && seg.ExitSpeed < seg.EntrySpeed {
		s = clamp((seg.EntrySpeed-seg.ExitSpeed)/seg.EntrySpeed/0.42, 0, 1)
	}
	down := clamp(-seg.Gradient/10, 0, 1)
	s *= 1 + 0.55*down
	if seg.Radius > 0 {
		s += 0.30 * down
	}
	return clamp(s, 0, 1.2)
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
	if s.IsFactory() {
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
		short := s.Gearing / 2 // +1 shortest, -1 longest
		// A car with power to spare pulls long gears happily and gains little
		// from short ones; an underpowered car lives on the short end.
		pull := clamp(1.45-0.55*power, 0.35, 1.45)
		reach := clamp(0.55+0.55*power, 0.55, 1.75)
		mod += setupGain * (0.034*short*crawling(seg)*pull - 0.030*short*nearLimiter(seg)*reach)
	}

	// --- Boost: power where it lands, lag where it does not ---------------
	if s.Boost != 0 {
		bo := s.Boost / 2
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
		soft := -s.Pressure / 2 // low pressure is the soft end
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
		fwd := s.Ballast / 2
		dt := b.Car.Drivetrain
		mod += setupGain * ballastLayout(dt) *
			(0.032*fwd*stopping(seg)*(1-transferSaturation*alreadyForward(seg)) -
				0.030*fwd*crawling(seg)*tractionAxle(dt)*drivenAxleGrip(seg, dt))
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
		agg := s.Camber / 2
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
		stiff := s.Damping / 2
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

// alreadyForward is how much of the car's weight the road itself has thrown
// onto the front axle.
//
// Ballast and gravity do the same job, and they do not add up twice. Braking
// into a corner on a one-in-ten descent has already loaded the front to the
// point where the tyre is the limit rather than the load, so the next kilo
// moved forward buys progressively less stopping power — and takes it off a
// rear axle that is light exactly when it can least afford to be. That is why
// a descent is not simply a road that wants all the mass forward: the steeper
// it is, the more the argument has already been settled without the spanner.
const transferSaturation = 0.40

func alreadyForward(seg Segment) float64 {
	return clamp(-seg.Gradient/12, 0, 1)
}

// drivenAxleGrip is how much the driven wheels are relying on where the mass
// has been put, given what the road has already done to them.
//
// Gravity unloads the rear of a car going down a mountain and the front of one
// going up it. Take mass off an axle that is already light and it spins; take
// it off one that gravity is pressing into the road and it barely notices. So
// the same kilo is worth more or less depending on which way the road points
// and which end drives — a rear-driven car feels the descent, a front-driven
// one feels the climb.
//
// This is the interaction that was missing while half the passes were
// layout-blind. Without it a descent is only a braking problem, every layout
// wants its mass forward for the same reason, and the driven axle never gets a
// vote on the roads where it should have the loudest one.
const axleSlopeGain = 0.55

func drivenAxleGrip(seg Segment, dt Drivetrain) float64 {
	var rearDriven float64
	switch dt {
	case FF:
		// Half of what a rear-driven car feels, and for a reason rather than
		// for symmetry: traction matters at the exit, and there the car's own
		// acceleration is transferring load off the front again — cancelling
		// part of what the slope just handed it.
		rearDriven = -0.5
	case FR:
		rearDriven = 1
	case MR:
		rearDriven = 1.15
	case AWD:
		rearDriven = 0.15
	}
	// Positive downhill, negative up.
	slope := clamp(-seg.Gradient/12, -1, 1)
	return clamp(1+axleSlopeGain*rearDriven*slope, 0.4, 1.8)
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
	if s.IsFactory() {
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

	v.SigmaMult += s.Boost * 0.100
	v.MistakeMult += s.Boost * 0.015

	v.SigmaMult += s.Camber * 0.030
	v.MistakeMult += s.Camber * 0.030

	v.SigmaMult += s.Damping * 0.060
	v.MistakeMult += s.Damping * 0.010

	v.SigmaMult -= s.Pressure * 0.025

	// A car on the floor has no travel left to absorb a mistake.
	v.MistakeMult += s.Ride * 0.020

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
	a += 0.42 * clamp(s.Ride/2, 0, 1)      // slammed
	a += 0.34 * clamp(s.Damping/2, 0, 1)   // no compliance
	a += 0.30 * clamp(s.Camber/2, 0, 1)    // on the edge of the tyre
	a += 0.26 * clamp(s.Boost/2, 0, 1)     // sudden delivery
	a += 0.18 * clamp(-s.Pressure/2, 0, 1) // soft and squirming
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
	if s.IsFactory() {
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
				0.032 * s.Damping / 2 * rough(seg) * heavy})
		}
		if s.Camber > 0 && stopping(seg) > 0.55 {
			// A corner is braked *for*; a straight is braked *at the end of*.
			// Same term either way, but "under braking for the Middle
			// Straight" reads like the road was misfiled.
			where := "under braking for " + seg.Name
			if seg.Radius <= 0 {
				where = "braking at the end of " + seg.Name
			}
			notes = append(notes, SetupNote{seg.Name,
				"Not enough tyre on the road " + where + ".",
				0.036 * s.Camber / 2 * stopping(seg) * (1 + 0.5*slick)})
		}
		if s.Boost > 0 && crawling(seg) > 0.5 {
			notes = append(notes, SetupNote{seg.Name,
				"You waited for the turbo on the way out of " + seg.Name + ".",
				0.028 * s.Boost / 2 * crawling(seg) * (1 + 1.1*slick)})
		}
		if s.Pressure < 0 && heat(seg, progress) > 0.45 {
			notes = append(notes, SetupNote{seg.Name,
				"The tyres were long gone by " + seg.Name + ".",
				0.030 * -s.Pressure / 2 * heat(seg, progress) * heavy * (1 - 0.8*slick)})
		}
		if s.Gearing > 0 && nearLimiter(seg) > 0.5 {
			notes = append(notes, SetupNote{seg.Name,
				"Bouncing off the limiter down " + seg.Name + ".",
				0.030 * s.Gearing / 2 * nearLimiter(seg)})
		}
		if s.Gearing < 0 && crawling(seg) > 0.55 {
			notes = append(notes, SetupNote{seg.Name,
				"Nothing there off the corner in " + seg.Name + ".",
				0.034 * -s.Gearing / 2 * crawling(seg)})
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
