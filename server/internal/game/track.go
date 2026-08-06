package game

// SegmentKind determines which stats matter through a piece of road.
type SegmentKind string

const (
	KindStraight  SegmentKind = "straight"
	KindSweeper   SegmentKind = "sweeper"   // fast, committed corner
	KindHairpin   SegmentKind = "hairpin"   // slow, rotation and braking
	KindTechnical SegmentKind = "technical" // linked S-curves
	KindBraking   SegmentKind = "braking"   // heavy braking zone
	KindChicane   SegmentKind = "chicane"   // quick direction change
)

// SegWeights splits a segment's difficulty across car and driver attributes.
// Each group sums to 1 so a reference build scores exactly 100.
type SegWeights struct {
	// car
	Power, Grip, Handling, Braking float64
	// driver
	Bravery, Intelligence, Reflexes, Consistency, Stamina float64
}

var segmentWeights = map[SegmentKind]SegWeights{
	KindStraight: {
		Power: 0.78, Grip: 0.08, Handling: 0.06, Braking: 0.08,
		Bravery: 0.30, Intelligence: 0.20, Reflexes: 0.10, Consistency: 0.25, Stamina: 0.15,
	},
	KindSweeper: {
		Power: 0.22, Grip: 0.46, Handling: 0.22, Braking: 0.10,
		Bravery: 0.40, Intelligence: 0.18, Reflexes: 0.14, Consistency: 0.18, Stamina: 0.10,
	},
	KindHairpin: {
		Power: 0.14, Grip: 0.24, Handling: 0.38, Braking: 0.24,
		Bravery: 0.16, Intelligence: 0.22, Reflexes: 0.38, Consistency: 0.16, Stamina: 0.08,
	},
	KindTechnical: {
		Power: 0.10, Grip: 0.28, Handling: 0.46, Braking: 0.16,
		Bravery: 0.12, Intelligence: 0.34, Reflexes: 0.30, Consistency: 0.16, Stamina: 0.08,
	},
	KindBraking: {
		Power: 0.10, Grip: 0.26, Handling: 0.14, Braking: 0.50,
		Bravery: 0.42, Intelligence: 0.18, Reflexes: 0.18, Consistency: 0.14, Stamina: 0.08,
	},
	KindChicane: {
		Power: 0.12, Grip: 0.30, Handling: 0.40, Braking: 0.18,
		Bravery: 0.18, Intelligence: 0.20, Reflexes: 0.32, Consistency: 0.22, Stamina: 0.08,
	},
}

// drivetrainMod nudges a segment's performance by layout. Small numbers on
// purpose — it is flavour, not a hard counter.
func drivetrainMod(kind SegmentKind, dt Drivetrain) float64 {
	switch dt {
	case AWD:
		switch kind {
		case KindStraight, KindSweeper:
			return 1.03
		case KindHairpin, KindChicane:
			return 0.975
		}
	case FF:
		switch kind {
		case KindHairpin:
			return 0.96
		case KindTechnical:
			return 1.01
		case KindBraking:
			return 0.99
		}
	case MR:
		switch kind {
		case KindTechnical, KindChicane:
			return 1.03
		case KindSweeper:
			return 0.985
		}
	case FR:
		switch kind {
		case KindHairpin:
			return 1.025
		case KindStraight:
			return 0.995
		}
	}
	return 1.0
}

type Segment struct {
	Name string      `json:"name"`
	Kind SegmentKind `json:"kind"`
	// BaseSeconds is how long a reference build (rating 100) takes.
	BaseSeconds float64 `json:"base_seconds"`
	// Risk 0..1 scales both random noise and mistake probability.
	Risk float64 `json:"risk"`
}

type TrackDef struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Flavour  string    `json:"flavour"`
	Downhill bool      `json:"downhill"`
	Segments []Segment `json:"segments"`
}

func (t TrackDef) BaseSeconds() float64 {
	var s float64
	for _, seg := range t.Segments {
		s += seg.BaseSeconds
	}
	return s
}

var Tracks = []TrackDef{
	{
		ID: "akina_down", Name: "Mt. Akina — Downhill", Downhill: true,
		Flavour: "Five consecutive hairpins. The gutter is a line, if you're mad enough.",
		Segments: []Segment{
			{Name: "Start Line Chute", Kind: KindStraight, BaseSeconds: 7.5, Risk: 0.10},
			{Name: "First Right", Kind: KindSweeper, BaseSeconds: 5.5, Risk: 0.30},
			{Name: "Consecutive Hairpin 1", Kind: KindHairpin, BaseSeconds: 6.2, Risk: 0.55},
			{Name: "Consecutive Hairpin 2", Kind: KindHairpin, BaseSeconds: 6.0, Risk: 0.55},
			{Name: "Consecutive Hairpin 3", Kind: KindHairpin, BaseSeconds: 6.1, Risk: 0.60},
			{Name: "Consecutive Hairpin 4", Kind: KindHairpin, BaseSeconds: 6.0, Risk: 0.60},
			{Name: "Consecutive Hairpin 5", Kind: KindHairpin, BaseSeconds: 6.3, Risk: 0.65},
			{Name: "Downhill Straight", Kind: KindStraight, BaseSeconds: 9.0, Risk: 0.12},
			{Name: "Blind Left", Kind: KindTechnical, BaseSeconds: 5.8, Risk: 0.45},
			{Name: "The Gutter Corner", Kind: KindTechnical, BaseSeconds: 5.4, Risk: 0.75},
			{Name: "Final Braking Zone", Kind: KindBraking, BaseSeconds: 5.0, Risk: 0.50},
			{Name: "Run to the Line", Kind: KindStraight, BaseSeconds: 8.2, Risk: 0.10},
		},
	},
	{
		ID: "myogi_up", Name: "Myogi — Uphill",
		Flavour: "Short, tight, and the crowd stands close enough to touch the mirrors.",
		Segments: []Segment{
			{Name: "Launch", Kind: KindStraight, BaseSeconds: 6.0, Risk: 0.15},
			{Name: "Tight Right", Kind: KindHairpin, BaseSeconds: 5.4, Risk: 0.45},
			{Name: "Uphill Esses", Kind: KindTechnical, BaseSeconds: 7.2, Risk: 0.40},
			{Name: "Short Chute", Kind: KindStraight, BaseSeconds: 5.0, Risk: 0.10},
			{Name: "Bumpy Chicane", Kind: KindChicane, BaseSeconds: 4.6, Risk: 0.55},
			{Name: "Second Hairpin", Kind: KindHairpin, BaseSeconds: 5.6, Risk: 0.48},
			{Name: "Long Left Sweeper", Kind: KindSweeper, BaseSeconds: 6.4, Risk: 0.35},
			{Name: "Crest Braking", Kind: KindBraking, BaseSeconds: 4.4, Risk: 0.52},
			{Name: "Final Esses", Kind: KindTechnical, BaseSeconds: 6.0, Risk: 0.42},
			{Name: "Sprint to the Line", Kind: KindStraight, BaseSeconds: 7.0, Risk: 0.12},
		},
	},
	{
		ID: "irohazaka", Name: "Irohazaka — One Way Down", Downhill: true,
		Flavour: "Forty-eight corners, all numbered, all going the same direction. Down.",
		Segments: []Segment{
			{Name: "Gate Straight", Kind: KindStraight, BaseSeconds: 6.8, Risk: 0.10},
			{Name: "Corner 1 — Ha", Kind: KindHairpin, BaseSeconds: 5.8, Risk: 0.50},
			{Name: "Corner 2 — Ni", Kind: KindHairpin, BaseSeconds: 5.6, Risk: 0.52},
			{Name: "Double Apex", Kind: KindTechnical, BaseSeconds: 6.6, Risk: 0.48},
			{Name: "Camber Change", Kind: KindChicane, BaseSeconds: 5.0, Risk: 0.60},
			{Name: "Long Descent", Kind: KindStraight, BaseSeconds: 8.4, Risk: 0.14},
			{Name: "Brake Fade Zone", Kind: KindBraking, BaseSeconds: 5.6, Risk: 0.68},
			{Name: "Tightening Right", Kind: KindHairpin, BaseSeconds: 5.9, Risk: 0.58},
			{Name: "Blind Crest", Kind: KindSweeper, BaseSeconds: 6.2, Risk: 0.62},
			{Name: "Last Hairpin", Kind: KindHairpin, BaseSeconds: 5.7, Risk: 0.55},
			{Name: "Exit Road", Kind: KindStraight, BaseSeconds: 7.6, Risk: 0.10},
		},
	},
	{
		ID: "happogahara", Name: "Happogahara — High Speed",
		Flavour: "Wide, fast, and it rewards the brave over the clever.",
		Segments: []Segment{
			{Name: "Rolling Start", Kind: KindStraight, BaseSeconds: 9.5, Risk: 0.10},
			{Name: "Flat-Out Kink", Kind: KindSweeper, BaseSeconds: 6.8, Risk: 0.45},
			{Name: "Big Braking", Kind: KindBraking, BaseSeconds: 5.2, Risk: 0.42},
			{Name: "Fast Left", Kind: KindSweeper, BaseSeconds: 7.0, Risk: 0.40},
			{Name: "Back Straight", Kind: KindStraight, BaseSeconds: 10.5, Risk: 0.08},
			{Name: "Late Braking Duel", Kind: KindBraking, BaseSeconds: 5.4, Risk: 0.58},
			{Name: "Sweeping Right", Kind: KindSweeper, BaseSeconds: 7.2, Risk: 0.38},
			{Name: "Technical Section", Kind: KindTechnical, BaseSeconds: 6.4, Risk: 0.40},
			{Name: "Final Kink", Kind: KindSweeper, BaseSeconds: 6.0, Risk: 0.35},
			{Name: "Drag to the Line", Kind: KindStraight, BaseSeconds: 9.8, Risk: 0.08},
		},
	},
	{
		ID: "akagi_down", Name: "Mt. Akagi — Downhill", Downhill: true,
		Flavour: "Rhythm section after rhythm section. Break the rhythm, lose the race.",
		Segments: []Segment{
			{Name: "Summit Start", Kind: KindStraight, BaseSeconds: 7.0, Risk: 0.10},
			{Name: "Opening Sweeper", Kind: KindSweeper, BaseSeconds: 6.0, Risk: 0.32},
			{Name: "Rhythm Esses 1", Kind: KindTechnical, BaseSeconds: 6.5, Risk: 0.44},
			{Name: "Rhythm Esses 2", Kind: KindTechnical, BaseSeconds: 6.3, Risk: 0.46},
			{Name: "Downhill Chicane", Kind: KindChicane, BaseSeconds: 4.8, Risk: 0.58},
			{Name: "Guardrail Hairpin", Kind: KindHairpin, BaseSeconds: 5.9, Risk: 0.62},
			{Name: "Middle Straight", Kind: KindStraight, BaseSeconds: 8.6, Risk: 0.12},
			{Name: "Off-Camber Left", Kind: KindTechnical, BaseSeconds: 5.7, Risk: 0.66},
			{Name: "Heavy Braking", Kind: KindBraking, BaseSeconds: 5.1, Risk: 0.50},
			{Name: "Final Hairpin", Kind: KindHairpin, BaseSeconds: 6.0, Risk: 0.54},
			{Name: "Run Out", Kind: KindStraight, BaseSeconds: 8.0, Risk: 0.10},
		},
	},
	{
		ID: "usui", Name: "Usui Pass — Downhill", Downhill: true,
		Flavour: "Narrow, damp, and lined with a hundred and eighty corners of history.",
		Segments: []Segment{
			{Name: "Cold Start", Kind: KindStraight, BaseSeconds: 6.4, Risk: 0.12},
			{Name: "Damp Right", Kind: KindSweeper, BaseSeconds: 5.8, Risk: 0.50},
			{Name: "Narrow Esses", Kind: KindTechnical, BaseSeconds: 6.8, Risk: 0.52},
			{Name: "Stone Wall Hairpin", Kind: KindHairpin, BaseSeconds: 6.1, Risk: 0.70},
			{Name: "Short Straight", Kind: KindStraight, BaseSeconds: 6.6, Risk: 0.12},
			{Name: "Double Chicane", Kind: KindChicane, BaseSeconds: 5.2, Risk: 0.60},
			{Name: "Mossy Braking Zone", Kind: KindBraking, BaseSeconds: 5.0, Risk: 0.64},
			{Name: "Tunnel Sweeper", Kind: KindSweeper, BaseSeconds: 6.2, Risk: 0.44},
			{Name: "Last Esses", Kind: KindTechnical, BaseSeconds: 6.4, Risk: 0.48},
			{Name: "Bridge Straight", Kind: KindStraight, BaseSeconds: 7.4, Risk: 0.10},
		},
	},
}

func TrackByID(id string) (TrackDef, bool) {
	for _, t := range Tracks {
		if t.ID == id {
			return t, true
		}
	}
	return TrackDef{}, false
}
