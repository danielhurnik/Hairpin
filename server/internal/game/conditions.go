package game

// ---------------------------------------------------------------------------
// Conditions
//
// The pass is never quite the same twice. This is the strongest thing standing
// between the game and a solved setup: a sheet worked out on a dry evening is
// the wrong sheet on a cold damp one, so there is no answer to write down and
// pass around — only a way of reading what you have been given.
//
// Like the track itself, conditions are a pure function of who you are and how
// many races you have finished. They are fixed before you tune and revealed
// with the road, so it is a decision rather than a gamble; they simply are not
// the same decision twice.
// ---------------------------------------------------------------------------

type Conditions struct {
	// Damp runs 0 (bone dry) to 1 (streaming). Most evenings are near zero.
	Damp float64 `json:"damp"`
	// TempC is the surface temperature. Cold rubber does not work.
	TempC float64 `json:"temp_c"`
	Label string  `json:"label"`
	Note  string  `json:"note"`
}

// Dry is the reference condition — what every rating and every calibration is
// quoted against.
func Dry() Conditions {
	return Conditions{Damp: 0, TempC: 18, Label: "Dry", Note: "Warm asphalt, good grip."}
}

// ConditionsFor is the weather waiting for a player on their next race.
func ConditionsFor(playerID string, racesCompleted int, t TrackDef) Conditions {
	// A separate hash from the track's, so the same road turns up in different
	// weather rather than always arriving with its own.
	var h uint64 = 1469598103934665603
	for i := 0; i < len(playerID); i++ {
		h ^= uint64(playerID[i])
		h *= 1099511628211
	}
	h ^= uint64(racesCompleted)*0x9E3779B97F4A7C15 + 0x632BE59BD9B4E019
	h *= 1099511628211
	h ^= h >> 31
	h *= 0x7FB5D329728EA185
	h ^= h >> 27

	roll := float64(h%10000) / 10000
	tempRoll := float64((h>>17)%10000) / 10000

	c := Conditions{TempC: 4 + tempRoll*22}

	// Mountain passes are damp more often than not in the small hours, and
	// Usui is a byword for it.
	wetBias := 0.0
	if t.ID == "usui" {
		wetBias = 0.18
	}
	if t.Downhill {
		wetBias += 0.05
	}

	switch r := roll - wetBias; {
	case r < 0.42:
		c.Damp = 0
	case r < 0.66:
		c.Damp = 0.22
	case r < 0.85:
		c.Damp = 0.5
	default:
		c.Damp = 0.8
	}

	c.Label, c.Note = describeConditions(c)
	return c
}

func describeConditions(c Conditions) (string, string) {
	cold := c.TempC < 10
	switch {
	case c.Damp >= 0.75 && cold:
		return "Cold and streaming", "Standing water and no temperature in the rubber. Everything happens slowly, or all at once."
	case c.Damp >= 0.75:
		return "Wet", "Water across the camber. Grip is somewhere, but not where you left it."
	case c.Damp >= 0.45 && cold:
		return "Cold and wet", "Greasy under the trees, drying on the exposed sections."
	case c.Damp >= 0.45:
		return "Damp", "The road never dried out. Careful with the throttle."
	case c.Damp > 0 && cold:
		return "Cold, damp patches", "Dry line, wet edges, and tyres that would rather be at home."
	case c.Damp > 0:
		return "Damp patches", "Mostly dry, with the odd wet stripe where the trees overhang."
	case cold:
		return "Cold and dry", "Dry, but the tyres will take a while to come to you."
	}
	return "Dry", "Warm asphalt, good grip."
}

// ApplyConditions stamps the weather onto a copy of the track, so everything
// downstream — corner speeds, risk, what the setup wants — reads it off the
// road without needing to be handed the forecast separately.
func ApplyConditions(t TrackDef, c Conditions) TrackDef {
	out := t
	out.Segments = make([]Segment, len(t.Segments))
	copy(out.Segments, t.Segments)

	// Cold rubber is its own kind of slippery, and it never fully goes away
	// on a two-minute run down a mountain.
	cold := clamp((14-c.TempC)/12, 0, 1) * 0.35

	for i := range out.Segments {
		seg := &out.Segments[i]
		seg.Wet = clamp(c.Damp, 0, 1)
		seg.Cold = cold

		slip := c.Damp*0.85 + cold*0.5
		// A smooth resurfaced road is the worst of the lot when it is wet;
		// coarse chippings still find you some grip.
		if seg.Surface == SurfaceSmooth {
			slip *= 1.15
		}
		if seg.Surface == SurfaceCoarse {
			slip *= 0.85
		}
		seg.Risk = clamp(seg.Risk*(1+0.45*slip), 0.05, 0.98)
	}
	return out
}
