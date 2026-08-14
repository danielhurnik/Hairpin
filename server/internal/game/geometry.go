package game

import "math"

// ---------------------------------------------------------------------------
// Road geometry
//
// A corner used to be a type, a duration and a risk number. That is enough to
// decide who is faster and nowhere near enough to decide what to do with the
// car: "hairpin, 6.2 seconds" gives a setup nothing to bite on, which is why
// tuning could only ever be a nudge.
//
// So the road is described instead of labelled. A corner has a radius, a
// width, a camber, a gradient and a surface, and everything the game used to
// be told — how long it takes, how dangerous it is, what kind of corner it
// even is — is derived from those. Lower the car and it is the crest at the
// bottom of the descent that punishes you, not an abstraction.
//
// ON ACCURACY: these are authored approximations, not survey data. They are
// built to match what each pass is known for — Irohazaka's forty-eight
// one-way hairpins, Usui's damp stone-walled narrowness, Akina's five in a
// row — and to be internally consistent under the model below. Nobody should
// mistake a radius here for a measured one.
// ---------------------------------------------------------------------------

type Surface string

const (
	// SurfaceSmooth is resurfaced asphalt: fast, predictable, less grip when cold.
	SurfaceSmooth Surface = "smooth"
	// SurfaceCoarse is old chipped asphalt: mechanical grip, hard on tyres.
	SurfaceCoarse Surface = "coarse"
	// SurfacePatched is a road repaired in strips, so grip changes mid-corner.
	SurfacePatched Surface = "patched"
	// SurfaceBroken is ruts, frost heave and edges that unsettle a stiff car.
	SurfaceBroken Surface = "broken"
)

// bumpAmplitude is roughly how far the road moves under the car, in
// millimetres. It is what a ride-height setting is measured against.
func (s Surface) bumpAmplitude() float64 {
	switch s {
	case SurfaceSmooth:
		return 18
	case SurfaceCoarse:
		return 34
	case SurfacePatched:
		return 55
	case SurfaceBroken:
		return 85
	}
	return 30
}

// gripFactor is the surface's contribution to available grip, dry.
func (s Surface) gripFactor() float64 {
	switch s {
	case SurfaceSmooth:
		return 0.98
	case SurfaceCoarse:
		return 1.05
	case SurfacePatched:
		return 0.94
	case SurfaceBroken:
		return 0.90
	}
	return 1.0
}

// Segment is one piece of road. The geometry is authored; everything from
// Kind down is derived from it by deriveTrack at start-up.
type Segment struct {
	Name string `json:"name"`

	// --- geometry, authored ---

	// Radius of the corner in metres. Zero means straight.
	Radius float64 `json:"radius_m,omitempty"`
	// Length of road covered, in metres.
	Length float64 `json:"length_m"`
	// Width of the carriageway in metres. Narrow roads punish a mistake that a
	// wide one would forgive, and leave less room to run a different line.
	Width float64 `json:"width_m"`
	// Banking in degrees. Positive is banked into the turn and helps; negative
	// is off-camber and is where cars leave the road.
	Banking float64 `json:"banking_deg"`
	// Gradient as a percentage. Negative is downhill, which lengthens braking
	// and unloads the rear.
	Gradient float64 `json:"gradient_pct"`
	Surface  Surface `json:"surface"`
	// Crest marks a brow or a compression: the car goes light, or bottoms out.
	Crest bool `json:"crest,omitempty"`
	// Blind means the exit cannot be seen on entry, which is worth an
	// intelligent driver something and a brave one nothing.
	Blind bool `json:"blind,omitempty"`
	// Dir is -1 for a left, +1 for a right, 0 for a straight. Consecutive
	// corners that alternate are a rhythm section; ones that do not are a
	// long single load the tyres have to live through.
	Dir int `json:"direction"`

	// --- derived, do not author ---

	Kind SegmentKind `json:"kind"`
	// BaseSeconds is how long a reference build takes, worked out from the
	// geometry rather than guessed.
	BaseSeconds float64 `json:"base_seconds"`
	// Risk 0..1 scales noise and mistakes. Falls out of width, surface,
	// camber, sight and how much grip the corner is already asking for.
	Risk float64 `json:"risk"`
	// EntrySpeed and ExitSpeed are m/s for a reference car, from the speed
	// pass below. They are what makes a long straight different from a short
	// one to a set of gears.
	EntrySpeed float64 `json:"entry_speed"`
	ExitSpeed  float64 `json:"exit_speed"`
	// LateralG is how hard the reference car is leaning on the tyres here.
	// Sustained load is what camber and pressure actually trade against.
	LateralG float64 `json:"lateral_g"`
	// Rhythm is true when this corner reverses the previous one's direction.
	Rhythm bool `json:"rhythm,omitempty"`
}

// Reference physics. These set the scale of the whole game: a reference car
// is a stock coupe with an average driver, and every corner speed and segment
// duration in the game comes out of these five numbers.
const (
	gravity = 9.81
	// refGrip is the reference car's lateral grip coefficient, dry.
	refGrip = 1.02
	// refAccel and refBrake are longitudinal, in m/s².
	refAccel = 3.4
	refBrake = 8.6
	// refTopSpeed caps a straight, in m/s (about 200 km/h).
	refTopSpeed = 55.0
)

// cornerSpeed is the fastest a reference car gets through a corner, from the
// radius and how the road is leaning.
//
// Banking is added to the grip budget rather than multiplied into it, which is
// the usual flat-road approximation and is what makes an off-camber corner
// genuinely frightening: take enough away and there is very little left.
func cornerSpeed(seg Segment) float64 {
	if seg.Radius <= 0 {
		return refTopSpeed
	}
	mu := refGrip*seg.Surface.gripFactor() + math.Tan(seg.Banking*math.Pi/180)
	if mu < 0.18 {
		mu = 0.18
	}
	v := math.Sqrt(mu * gravity * seg.Radius)
	return math.Min(v, refTopSpeed)
}

// deriveAll fills in every track's derived fields and hands them back, so the
// Tracks var is already complete by the time anything else initialises.
func deriveAll(ts []TrackDef) []TrackDef {
	for i := range ts {
		deriveTrack(&ts[i])
	}
	return ts
}

// deriveTrack fills in everything downstream of the geometry.
//
// The speed pass is the standard quasi-steady-state one: work out what each
// corner allows, then walk forwards limiting by how hard the car can
// accelerate and backwards limiting by how hard it can stop. It is used only
// to establish how long each piece of road takes a reference car — who is
// actually faster on the night is still the attribute model's business — but
// it means a tight corner is slow because it is tight.
func deriveTrack(t *TrackDef) {
	n := len(t.Segments)
	if n == 0 {
		return
	}

	limit := make([]float64, n)
	for i := range t.Segments {
		limit[i] = cornerSpeed(t.Segments[i])
	}

	// Forwards: you cannot arrive faster than you could accelerate to. The
	// first segment starts at a rolling pace rather than at terminal velocity,
	// so an opening straight is something gearing has to actually pull.
	v := make([]float64, n)
	v[0] = math.Min(limit[0], 16)
	for i := 1; i < n; i++ {
		prev := t.Segments[i-1]
		// Gravity helps downhill and hurts up.
		a := refAccel - gravity*prev.Gradient/100
		if a < 0.4 {
			a = 0.4
		}
		reach := math.Sqrt(v[i-1]*v[i-1] + 2*a*prev.Length)
		v[i] = math.Min(limit[i], reach)
	}

	// Backwards: you cannot carry more speed than you can shed before the next
	// corner. Downhill costs braking distance, which is the whole character of
	// a descent.
	for i := n - 2; i >= 0; i-- {
		seg := t.Segments[i]
		b := refBrake + gravity*seg.Gradient/100
		if b < 1.5 {
			b = 1.5
		}
		reach := math.Sqrt(v[i+1]*v[i+1] + 2*b*seg.Length)
		v[i] = math.Min(v[i], reach)
	}

	for i := range t.Segments {
		seg := &t.Segments[i]
		entry := v[i]
		exit := limit[i]
		if i+1 < n {
			exit = math.Min(limit[i], v[i+1])
		}
		if seg.Radius > 0 {
			// Through a corner the car is near its limit the whole way.
			exit = math.Min(exit, limit[i])
		}
		a := refAccel - gravity*seg.Gradient/100
		if a < 0.4 {
			a = 0.4
		}
		b := refBrake + gravity*seg.Gradient/100
		if b < 1.5 {
			b = 1.5
		}
		seg.EntrySpeed = entry
		seg.ExitSpeed = exit
		seg.BaseSeconds = travelTime(entry, exit, seg.Length, a, b, limit[i])
		avg := seg.Length / math.Max(seg.BaseSeconds, 0.01)

		if seg.Radius > 0 {
			seg.LateralG = (avg * avg) / (seg.Radius * gravity)
		}
		if i > 0 && seg.Dir != 0 && t.Segments[i-1].Dir != 0 && seg.Dir != t.Segments[i-1].Dir {
			seg.Rhythm = true
		}
		seg.Kind = deriveKind(t.Segments, i)
		seg.Risk = deriveRisk(*seg)
	}
}

// travelTime integrates one piece of road properly: accelerate away from the
// entry speed, hold whatever cap applies, then brake for the exit.
//
// Averaging entry and exit instead — which a first pass here did — makes a
// long straight between two slow corners look as slow as the corners. Akina's
// 340-metre descent came out at fifteen seconds when the car should be near
// two hundred km/h in the middle of it, which quietly made power and gearing
// worth far less than they are.
func travelTime(v0, v1, length, accel, brake, cap float64) float64 {
	if length <= 0 {
		return 0
	}
	v0 = math.Min(v0, cap)
	v1 = math.Min(v1, cap)

	// Where accelerating from the entry meets braking for the exit.
	d := (v1*v1 - v0*v0 + 2*brake*length) / (2 * (accel + brake))
	d = clamp(d, 0, length)
	peak := math.Sqrt(math.Max(v0*v0+2*accel*d, 0))

	if peak <= cap {
		// A pure accelerate-then-brake segment, no cruising.
		tAcc := (peak - v0) / accel
		tBrk := (peak - v1) / brake
		return math.Max(tAcc, 0) + math.Max(tBrk, 0)
	}

	// Capped: accelerate to the cap, hold it, then brake.
	dAcc := math.Max((cap*cap-v0*v0)/(2*accel), 0)
	dBrk := math.Max((cap*cap-v1*v1)/(2*brake), 0)
	cruise := length - dAcc - dBrk
	if cruise < 0 {
		// Not enough road to reach the cap after all; fall back to the
		// uncapped solution, which is exact in that case.
		tAcc := (peak - v0) / accel
		tBrk := (peak - v1) / brake
		return math.Max(tAcc, 0) + math.Max(tBrk, 0)
	}
	return (cap-v0)/accel + cruise/cap + (cap-v1)/brake
}

// deriveKind works out what sort of corner this is from its shape and from
// what leads into it, so the existing per-kind attribute weights keep working
// against geometry rather than against an author's label.
func deriveKind(segs []Segment, i int) SegmentKind {
	seg := segs[i]
	if seg.Radius <= 0 {
		return KindStraight
	}
	// A big speed drop on entry makes this a braking zone whatever its radius.
	if i > 0 {
		prev := segs[i-1]
		if prev.ExitSpeed > 0 && seg.EntrySpeed > 0 {
			if drop := (prev.ExitSpeed - seg.EntrySpeed) / prev.ExitSpeed; drop > 0.34 {
				return KindBraking
			}
		}
	}
	switch {
	case seg.Radius < 24:
		return KindHairpin
	case seg.Rhythm && seg.Radius < 70:
		return KindChicane
	case seg.Radius < 65:
		return KindTechnical
	}
	return KindSweeper
}

// deriveRisk turns the road into a number between 0 and 1. Everything in here
// is a reason a corner bites: no room, no grip, no camber, no view, and
// already asking a lot of the tyres.
func deriveRisk(seg Segment) float64 {
	risk := 0.08

	// Narrow roads. Eight metres is comfortable, five is a stone wall either side.
	risk += 0.26 * clamp((9-seg.Width)/4.5, 0, 1)

	switch seg.Surface {
	case SurfaceCoarse:
		risk += 0.04
	case SurfacePatched:
		risk += 0.12
	case SurfaceBroken:
		risk += 0.20
	}

	// Off-camber is the single most common way to lose a car on a pass.
	risk += 0.22 * clamp(-seg.Banking/5, 0, 1)
	// Steep descents.
	risk += 0.14 * clamp(-seg.Gradient/10, 0, 1)

	if seg.Blind {
		risk += 0.12
	}
	if seg.Crest {
		risk += 0.10
	}
	// Corners already near the limit have nothing spare.
	risk += 0.18 * clamp(seg.LateralG/1.15, 0, 1)

	return clamp(risk, 0.05, 0.95)
}

// TrackLength is the pass in metres.
func (t TrackDef) TrackLength() float64 {
	var m float64
	for _, s := range t.Segments {
		m += s.Length
	}
	return m
}

// Descent is the total elevation lost, in metres, which is what makes brake
// fade a real thing on the long one-way descents.
func (t TrackDef) Descent() float64 {
	var d float64
	for _, s := range t.Segments {
		if s.Gradient < 0 {
			d += -s.Gradient / 100 * s.Length
		}
	}
	return d
}
