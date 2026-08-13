package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// SetupAxisView is one row of the setup sheet: what it is, whether the car has
// the part for it, where it is set, and how that suits the road ahead.
//
// What it deliberately does not carry is a number. The player is told the
// shape of each trade and how much this particular pass cares, and is left to
// work out the rest — a screen that printed coefficients would be a solver,
// and the setup would stop being a decision.
type SetupAxisView struct {
	game.SetupAxis
	Value    int  `json:"value"`
	Min      int  `json:"min"`
	Max      int  `json:"max"`
	Unlocked bool `json:"unlocked"`
	// Locked axes say what to buy, in the language of the car.
	Requires string `json:"requires,omitempty"`
	// Effect is how the current setting suits the upcoming track:
	// strong / slight / neutral / poor / wrong.
	Effect string `json:"effect"`
	// Matters is how much this track responds to this axis at all:
	// high / some / little. An axis that does not matter here is not broken.
	Matters string `json:"matters"`
}

// SetupView is the sheet for the car currently being driven, against the track
// it is about to be driven on.
type SetupView struct {
	ChassisID   string          `json:"chassis_id"`
	ChassisName string          `json:"chassis_name"`
	Track       game.TrackDef   `json:"track"`
	Axes        []SetupAxisView `json:"axes"`
	// AnyUnlocked is false for a stock car, which can adjust nothing at all.
	AnyUnlocked bool `json:"any_unlocked"`
}

// NextTrack is the road this player is pointed at. It is a pure function of
// who they are and how many races they have finished, so it is stable until
// they actually race and cannot be re-rolled by asking again.
func (s *Store) NextTrack(p *Player) game.TrackDef {
	return game.NextTrackFor(p.ID, p.Wins+p.Losses)
}

// SetupFor builds the sheet for a player's active car.
func (s *Store) SetupFor(ctx context.Context, p *Player) SetupView {
	track := s.NextTrack(p)
	chassis, _ := game.ChassisByID(p.ChassisID)
	live := game.LegalSetup(p.Setup, p.Tiers)

	view := SetupView{
		ChassisID:   p.ChassisID,
		ChassisName: chassis.Name,
		Track:       track,
		Axes:        make([]SetupAxisView, 0, len(game.SetupAxes)),
	}

	for _, a := range game.SetupAxes {
		unlocked := game.AxisUnlocked(a, p.Tiers)
		value, _ := live.Get(a.ID)
		row := SetupAxisView{
			SetupAxis: a,
			Value:     value,
			Min:       game.SetupMin,
			Max:       game.SetupMax,
			Unlocked:  unlocked,
			Effect:    "neutral",
			Matters:   matters(game.SetupSpread(a.ID, track, chassis.Drivetrain)),
		}
		if !unlocked {
			row.Requires = fmt.Sprintf("%s — %s tier %d",
				a.Part, game.UpgradeLabel(a.Category), a.MinTier)
		} else {
			row.Effect = game.HintFor(a.ID, value, track, chassis.Drivetrain).Effect
			view.AnyUnlocked = true
		}
		view.Axes = append(view.Axes, row)
	}
	return view
}

// matters buckets how much a track responds to an axis. The thresholds are
// coarse on purpose: "this pass does not care about your gearing" is useful,
// "0.0071" is not.
//
// They are set against what the model actually produces rather than guessed.
// Across the six tracks the spreads run from about 0.001 to 0.054; a first
// pass bucketed at 0.055 and duly reported "little" for every axis on every
// road, which is a screen that tells the player nothing.
func matters(spread float64) string {
	switch {
	case spread >= 0.028:
		return "high"
	case spread >= 0.010:
		return "some"
	}
	return "little"
}

// SetSetup adjusts one axis on the car being driven.
//
// Free, and as often as you like — the money went on the parts. What the
// server checks is that the part is actually fitted and that the value is one
// the workshop could reach, both read from its own tables rather than from the
// request.
func (s *Store) SetSetup(ctx context.Context, playerID, axisID string, value int) (*Player, error) {
	axis, ok := game.AxisByID(axisID)
	if !ok {
		return nil, ErrInvalid
	}
	if value < game.SetupMin || value > game.SetupMax {
		return nil, ErrInvalid
	}
	col, ok := setupColumn(axisID)
	if !ok {
		return nil, ErrInvalid
	}

	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if !game.AxisUnlocked(axis, p.Tiers) {
			return ErrPartMissing
		}
		if err := execGuarded(ctx, tx, ErrNotFound,
			fmt.Sprintf(`UPDATE player_cars SET %s = ? WHERE player_id = ? AND chassis_id = ?`, col),
			value, playerID, p.ChassisID); err != nil {
			return err
		}
		if err := touch(ctx, tx, playerID, s.Now()); err != nil {
			return err
		}
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, err
}

// ResetSetup puts the car back to how it left the workshop.
func (s *Store) ResetSetup(ctx context.Context, playerID string) (*Player, error) {
	var out *Player
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE player_cars
			SET setup_gearing = 0, setup_boost = 0, setup_pressure = 0, setup_ballast = 0,
			    setup_ride = 0, setup_camber = 0, setup_damping = 0
			WHERE player_id = ? AND chassis_id = ?`, playerID, p.ChassisID); err != nil {
			return err
		}
		out, err = getPlayer(ctx, tx, playerID)
		return err
	})
	return out, err
}

// setupColumn maps an axis to its column from a fixed whitelist. The request
// never reaches the SQL.
func setupColumn(axisID string) (string, bool) {
	switch axisID {
	case "gearing":
		return "setup_gearing", true
	case "boost":
		return "setup_boost", true
	case "pressure":
		return "setup_pressure", true
	case "ballast":
		return "setup_ballast", true
	case "ride_height":
		return "setup_ride", true
	case "camber":
		return "setup_camber", true
	case "damping":
		return "setup_damping", true
	}
	return "", false
}
