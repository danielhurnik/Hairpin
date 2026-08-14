package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// SetupAxisView is one row of the setup sheet: what it is, whether the car has
// the part for it, and exactly what it is set to.
//
// It carries the real number — 85mm, 2.4 bar, 4.30 — and no opinion whatsoever
// about whether that is the right one. An earlier version scored each axis
// against the upcoming pass, which is a solver: whoever worked out the best
// answer once would have worked it out for everybody. Precision belongs in the
// player's hands; the uncertainty belongs in the outcome. What the setup was
// worth is something the run tells you afterwards.
type SetupAxisView struct {
	game.SetupAxis
	Value int `json:"value"`
	Min   int `json:"min"`
	Max   int `json:"max"`
	// Setting is the real value at the current notch, and Display is it
	// formatted with its unit.
	Setting  float64 `json:"setting"`
	Display  string  `json:"display"`
	Unlocked bool    `json:"unlocked"`
	// Locked axes say what to buy, in the language of the car.
	Requires string `json:"requires,omitempty"`
}

// SetupView is the sheet for the car currently being driven, against the track
// it is about to be driven on.
type SetupView struct {
	ChassisID   string          `json:"chassis_id"`
	ChassisName string          `json:"chassis_name"`
	Track       game.TrackDef   `json:"track"`
	Conditions  game.Conditions `json:"conditions"`
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

// NextConditions is the weather waiting on that road. Pinned the same way, and
// shown before the car is set up: the pass is never the same twice, and being
// told which version of it you are getting is what makes tuning a decision
// rather than a coin toss.
func (s *Store) NextConditions(p *Player) game.Conditions {
	return game.ConditionsFor(p.ID, p.Wins+p.Losses, s.NextTrack(p))
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
		Conditions:  game.ConditionsFor(p.ID, p.Wins+p.Losses, track),
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
			Setting:   a.Setting(value),
			Display:   display(a, value),
			Unlocked:  unlocked,
		}
		if !unlocked {
			row.Requires = fmt.Sprintf("%s — %s tier %d",
				a.Part, game.UpgradeLabel(a.Category), a.MinTier)
		} else {
			view.AnyUnlocked = true
		}
		view.Axes = append(view.Axes, row)
	}
	return view
}

// display formats a setting the way it would be written on a setup sheet.
func display(a game.SetupAxis, notch int) string {
	v := strconv.FormatFloat(a.Setting(notch), 'f', a.Decimals, 64)
	if a.Unit == "" {
		return v
	}
	if a.Unit == "°" {
		return v + a.Unit
	}
	return v + " " + a.Unit
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
