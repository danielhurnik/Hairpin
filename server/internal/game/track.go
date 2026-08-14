package game

// SegmentKind determines which stats matter through a piece of road. It is no
// longer authored — deriveKind works it out from the corner's radius and what
// leads into it — but the attribute weights below still hang off it.
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
			return 1.02
		case KindStraight:
			return 0.995
		}
	}
	return 1.0
}

// TrackSource records where a track's geometry came from, because the two are
// not the same claim and the difference should not be buried in a comment.
type TrackSource string

const (
	// SourceAuthored means the geometry was written to match what the road is
	// known for. Internally consistent, plausible, and not measured.
	SourceAuthored TrackSource = "authored"
	// SourceSurveyed means radius, length, gradient and direction came out of
	// a recorded line down the real road via scripts/gen_tracks.py.
	SourceSurveyed TrackSource = "surveyed"
)

type TrackDef struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Flavour  string      `json:"flavour"`
	Downhill bool        `json:"downhill"`
	Source   TrackSource `json:"source"`
	// RealWorld names the road this is standing in for, so a survey can be
	// recorded against the right piece of tarmac.
	RealWorld string    `json:"real_world,omitempty"`
	Segments  []Segment `json:"segments"`
}

// Surveyed reports whether this track's geometry was measured.
func (t TrackDef) Surveyed() bool { return t.Source == SourceSurveyed }

func (t TrackDef) BaseSeconds() float64 {
	var s float64
	for _, seg := range t.Segments {
		s += seg.BaseSeconds
	}
	return s
}

// Tracks is every pass in the game, measured where anyone has measured it and
// authored where nobody has yet.
//
// The derivation is wrapped around the literal rather than run from init() on
// purpose: the setup tables are package-level vars computed from these
// durations, and Go initialises vars by dependency but runs init() afterwards.
// Deriving here means anything reading Tracks reads finished ones.
var Tracks = deriveAll(mergeSurveyed(authoredTracks, surveyedTracks))

// mergeSurveyed prefers a measured track over the authored stand-in with the
// same ID, and keeps the authored one for any road nobody has recorded yet.
//
// This is the whole switchover: run scripts/gen_tracks.py over a GPX of the
// real road and the pass it describes stops being an approximation, with no
// other change anywhere.
func mergeSurveyed(authored, surveyed []TrackDef) []TrackDef {
	if len(surveyed) == 0 {
		return authored
	}
	byID := make(map[string]TrackDef, len(surveyed))
	for _, t := range surveyed {
		t.Source = SourceSurveyed
		byID[t.ID] = t
	}
	out := make([]TrackDef, 0, len(authored))
	for _, a := range authored {
		if s, ok := byID[a.ID]; ok {
			// The measured road wins, but the things a GPS cannot know stay
			// with the version that was written by someone who chose them.
			if s.Flavour == "" {
				s.Flavour = a.Flavour
			}
			if s.RealWorld == "" {
				s.RealWorld = a.RealWorld
			}
			s.Downhill = a.Downhill
			out = append(out, s)
			delete(byID, a.ID)
			continue
		}
		out = append(out, a)
	}
	// A surveyed road with no authored counterpart is still a road.
	for _, t := range surveyed {
		if _, unused := byID[t.ID]; unused {
			out = append(out, t)
		}
	}
	return out
}

// authoredTracks are the stand-ins: geometry written to match what each road
// is known for, internally consistent under the race model, and not measured.
// Duration, risk and corner type are all derived from the geometry at start-up
// by deriveTrack — nothing below says how long a corner takes or how dangerous
// it is, because those are consequences rather than choices.
//
// Each is authored to disagree with itself: a setup that suits the hairpins is
// wrong for the fast section, so there is a compromise to find rather than an
// answer to look up.
//
// Any of these is replaced the moment its road is recorded — see
// scripts/gen_tracks.py and mergeSurveyed.
var authoredTracks = []TrackDef{
	{
		ID: "akina_down", Name: "Mt. Akina — Downhill", Downhill: true,
		Flavour:   "Five consecutive hairpins. The gutter is a line, if you're mad enough.",
		Source:    SourceAuthored,
		RealWorld: "Mt. Haruna — the prefectural road down the east face from Lake Haruna towards Takasaki, Gunma. \"Akina\" is the name the manga gave it.",
		Segments: []Segment{
			{Name: "Start Line Chute", Length: 260, Width: 7.0, Gradient: -6, Surface: SurfaceSmooth},
			{Name: "First Right", Radius: 95, Length: 120, Width: 6.8, Banking: 2.0, Gradient: -6, Surface: SurfaceSmooth, Dir: 1},
			{Name: "Consecutive Hairpin 1", Radius: 16, Length: 62, Width: 6.2, Banking: -1.0, Gradient: -9, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Consecutive Hairpin 2", Radius: 15, Length: 58, Width: 6.0, Banking: -1.5, Gradient: -9, Surface: SurfaceCoarse, Dir: 1},
			{Name: "Consecutive Hairpin 3", Radius: 14, Length: 56, Width: 5.8, Banking: -2.0, Gradient: -10, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Consecutive Hairpin 4", Radius: 15, Length: 58, Width: 5.8, Banking: -1.5, Gradient: -10, Surface: SurfacePatched, Dir: 1},
			{Name: "Consecutive Hairpin 5", Radius: 13, Length: 54, Width: 5.6, Banking: -2.5, Gradient: -10, Surface: SurfacePatched, Dir: -1},
			{Name: "Downhill Straight", Length: 340, Width: 7.2, Gradient: -8, Surface: SurfaceSmooth, Crest: true},
			{Name: "Blind Left", Radius: 44, Length: 95, Width: 6.4, Banking: -1.0, Gradient: -7, Surface: SurfaceSmooth, Blind: true, Dir: -1},
			{Name: "The Gutter Corner", Radius: 22, Length: 70, Width: 5.4, Banking: -3.5, Gradient: -8, Surface: SurfaceBroken, Blind: true, Dir: 1},
			{Name: "Final Braking Zone", Radius: 34, Length: 80, Width: 6.6, Banking: 1.0, Gradient: -5, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Run to the Line", Length: 300, Width: 7.4, Gradient: -3, Surface: SurfaceSmooth},
		},
	},
	{
		ID: "myogi_up", Name: "Myogi — Uphill",
		Flavour:   "Short, tight, and the crowd stands close enough to touch the mirrors.",
		Source:    SourceAuthored,
		RealWorld: "Mt. Myogi — the climb on the Gunma side, tight and low-speed with almost no run-off.",
		Segments: []Segment{
			{Name: "Launch", Length: 200, Width: 6.6, Gradient: 5, Surface: SurfaceCoarse},
			{Name: "Tight Right", Radius: 19, Length: 60, Width: 5.8, Banking: 1.0, Gradient: 7, Surface: SurfaceCoarse, Dir: 1},
			{Name: "Uphill Esses", Radius: 48, Length: 150, Width: 5.6, Banking: -1.0, Gradient: 8, Surface: SurfacePatched, Dir: -1},
			{Name: "Short Chute", Length: 145, Width: 6.0, Gradient: 6, Surface: SurfacePatched},
			{Name: "Bumpy Chicane", Radius: 30, Length: 74, Width: 5.4, Banking: -2.0, Gradient: 5, Surface: SurfaceBroken, Dir: 1},
			{Name: "Second Hairpin", Radius: 17, Length: 58, Width: 5.6, Banking: 0.5, Gradient: 8, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Long Left Sweeper", Radius: 88, Length: 170, Width: 6.4, Banking: 2.5, Gradient: 4, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Crest Braking", Radius: 40, Length: 78, Width: 6.0, Banking: -0.5, Gradient: 2, Surface: SurfaceCoarse, Crest: true, Blind: true, Dir: 1},
			{Name: "Final Esses", Radius: 42, Length: 140, Width: 5.8, Banking: -1.0, Gradient: 6, Surface: SurfacePatched, Dir: -1},
			{Name: "Sprint to the Line", Length: 230, Width: 6.8, Gradient: 3, Surface: SurfaceSmooth},
		},
	},
	{
		ID: "irohazaka", Name: "Irohazaka — One Way Down", Downhill: true,
		Flavour:   "Forty-eight corners, all numbered, all going the same direction. Down.",
		Source:    SourceAuthored,
		RealWorld: "Iroha-zaka, Nikko, Tochigi. Two one-way roads between Nikko and Lake Chuzenji; forty-eight curves between them, each signed with a character of the Iroha poem. This is the descending one.",
		Segments: []Segment{
			{Name: "Gate Straight", Length: 240, Width: 7.6, Gradient: -5, Surface: SurfaceSmooth},
			{Name: "Corner 1 — Ha", Radius: 18, Length: 64, Width: 7.2, Banking: 1.5, Gradient: -8, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Corner 2 — Ni", Radius: 17, Length: 62, Width: 7.0, Banking: 1.0, Gradient: -9, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Double Apex", Radius: 52, Length: 155, Width: 7.0, Banking: 0.5, Gradient: -8, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Camber Change", Radius: 36, Length: 90, Width: 6.6, Banking: -3.0, Gradient: -9, Surface: SurfacePatched, Dir: 1},
			{Name: "Long Descent", Length: 380, Width: 7.4, Gradient: -10, Surface: SurfaceSmooth},
			{Name: "Brake Fade Zone", Radius: 26, Length: 82, Width: 6.8, Banking: -1.0, Gradient: -11, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Tightening Right", Radius: 15, Length: 60, Width: 6.6, Banking: 0.5, Gradient: -9, Surface: SurfaceCoarse, Blind: true, Dir: 1},
			{Name: "Blind Crest", Radius: 78, Length: 140, Width: 7.0, Banking: 1.0, Gradient: -6, Surface: SurfaceSmooth, Crest: true, Blind: true, Dir: -1},
			{Name: "Last Hairpin", Radius: 16, Length: 60, Width: 6.8, Banking: 1.5, Gradient: -8, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Exit Road", Length: 280, Width: 7.8, Gradient: -4, Surface: SurfaceSmooth},
		},
	},
	{
		ID: "happogahara", Name: "Happogahara — High Speed",
		Flavour:   "Wide, fast, and it rewards the brave over the clever.",
		Source:    SourceAuthored,
		RealWorld: "Happogahara — a wide, fast road with a slow infield section. The one place in the game with room to make a mistake in.",
		Segments: []Segment{
			{Name: "Rolling Start", Length: 460, Width: 13.0, Gradient: 0, Surface: SurfaceSmooth},
			{Name: "Flat-Out Kink", Radius: 240, Length: 180, Width: 12.5, Banking: 3.0, Gradient: 0, Surface: SurfaceSmooth, Dir: 1},
			{Name: "Big Braking", Radius: 55, Length: 110, Width: 12.0, Banking: 1.5, Gradient: -1, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Fast Left", Radius: 165, Length: 200, Width: 12.0, Banking: 2.5, Gradient: 1, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Back Straight", Length: 540, Width: 13.5, Gradient: 0, Surface: SurfaceSmooth},
			// The infield is what stops this place being a single long
			// answer. Wide and fast everywhere else, it drops to a second-gear
			// corner on broken concrete, so the low, stiff, long-geared car
			// that owns the straights has to give something back here.
			{Name: "Late Braking Duel", Radius: 21, Length: 95, Width: 10.5, Banking: -1.0, Gradient: 0, Surface: SurfaceBroken, Dir: 1},
			{Name: "Sweeping Right", Radius: 185, Length: 210, Width: 12.5, Banking: 3.5, Gradient: 0, Surface: SurfaceSmooth, Dir: 1},
			{Name: "Technical Section", Radius: 34, Length: 165, Width: 9.0, Banking: -2.0, Gradient: 1, Surface: SurfacePatched, Crest: true, Dir: -1},
			{Name: "Final Kink", Radius: 210, Length: 175, Width: 12.5, Banking: 2.0, Gradient: 0, Surface: SurfaceSmooth, Dir: 1},
			{Name: "Drag to the Line", Length: 500, Width: 13.0, Gradient: 0, Surface: SurfaceSmooth},
		},
	},
	{
		ID: "akagi_down", Name: "Mt. Akagi — Downhill", Downhill: true,
		Flavour:   "Rhythm section after rhythm section. Break the rhythm, lose the race.",
		Source:    SourceAuthored,
		RealWorld: "Mt. Akagi — the descent on the south face, Gunma. Long linked rhythm sections.",
		Segments: []Segment{
			{Name: "Summit Start", Length: 250, Width: 7.4, Gradient: -5, Surface: SurfaceSmooth},
			{Name: "Opening Sweeper", Radius: 105, Length: 145, Width: 7.2, Banking: 2.0, Gradient: -6, Surface: SurfaceSmooth, Dir: 1},
			{Name: "Rhythm Esses 1", Radius: 58, Length: 150, Width: 6.8, Banking: 0.5, Gradient: -7, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Rhythm Esses 2", Radius: 54, Length: 145, Width: 6.8, Banking: 0.5, Gradient: -7, Surface: SurfaceCoarse, Dir: 1},
			{Name: "Downhill Chicane", Radius: 33, Length: 84, Width: 6.4, Banking: -1.5, Gradient: -8, Surface: SurfacePatched, Dir: -1},
			{Name: "Guardrail Hairpin", Radius: 17, Length: 62, Width: 6.0, Banking: -2.0, Gradient: -9, Surface: SurfaceCoarse, Dir: 1},
			{Name: "Middle Straight", Length: 350, Width: 7.2, Gradient: -7, Surface: SurfaceSmooth},
			{Name: "Off-Camber Left", Radius: 40, Length: 100, Width: 6.2, Banking: -4.5, Gradient: -8, Surface: SurfaceBroken, Blind: true, Dir: -1},
			{Name: "Heavy Braking", Radius: 30, Length: 82, Width: 6.6, Banking: 1.0, Gradient: -6, Surface: SurfaceCoarse, Dir: 1},
			{Name: "Final Hairpin", Radius: 16, Length: 60, Width: 6.2, Banking: 0.0, Gradient: -7, Surface: SurfaceCoarse, Dir: -1},
			{Name: "Run Out", Length: 300, Width: 7.4, Gradient: -4, Surface: SurfaceSmooth},
		},
	},
	{
		ID: "usui", Name: "Usui Pass — Downhill", Downhill: true,
		Flavour:   "Narrow, damp, and lined with a hundred and eighty corners of history.",
		Source:    SourceAuthored,
		RealWorld: "Usui Pass, on the old Route 18 between Yokokawa in Gunma and Karuizawa in Nagano. A hundred and eighty-odd numbered curves, narrow, wooded and rarely dry.",
		Segments: []Segment{
			{Name: "Cold Start", Length: 210, Width: 5.8, Gradient: -4, Surface: SurfacePatched},
			{Name: "Damp Right", Radius: 62, Length: 120, Width: 5.4, Banking: -1.0, Gradient: -6, Surface: SurfacePatched, Dir: 1},
			{Name: "Narrow Esses", Radius: 38, Length: 150, Width: 5.0, Banking: -1.5, Gradient: -7, Surface: SurfaceBroken, Dir: -1},
			{Name: "Stone Wall Hairpin", Radius: 14, Length: 56, Width: 4.6, Banking: -2.5, Gradient: -8, Surface: SurfaceBroken, Blind: true, Dir: 1},
			{Name: "Short Straight", Length: 190, Width: 5.6, Gradient: -5, Surface: SurfacePatched},
			{Name: "Double Chicane", Radius: 29, Length: 88, Width: 5.0, Banking: -2.0, Gradient: -6, Surface: SurfaceBroken, Dir: -1},
			{Name: "Mossy Braking Zone", Radius: 25, Length: 76, Width: 4.8, Banking: -3.0, Gradient: -7, Surface: SurfaceBroken, Blind: true, Dir: 1},
			{Name: "Tunnel Sweeper", Radius: 84, Length: 145, Width: 5.6, Banking: 1.5, Gradient: -4, Surface: SurfaceSmooth, Dir: -1},
			{Name: "Last Esses", Radius: 44, Length: 140, Width: 5.2, Banking: -1.0, Gradient: -5, Surface: SurfacePatched, Dir: 1},
			{Name: "Bridge Straight", Length: 240, Width: 6.0, Gradient: -2, Surface: SurfaceSmooth},
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
