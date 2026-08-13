package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

// Opponent is the async "bot": another player's saved build, frozen at the
// moment we read it. Nothing about them is live — they are never notified,
// never consulted, and their own progress is unaffected apart from rank.
type Opponent struct {
	PlayerID  string     `json:"player_id"`
	Name      string     `json:"name"`
	Rank      int        `json:"ladder_rank"`
	Level     int        `json:"driver_level"`
	IsBot     bool       `json:"is_bot"`
	ChassisID string     `json:"chassis_id"`
	Chassis   string     `json:"chassis_name"`
	LiveryID  string     `json:"livery_id"`
	Rating    float64    `json:"rating"`
	Build     game.Build `json:"build"`
	Items     []string   `json:"notable_items,omitempty"`
}

// findOpponent picks someone to race. Preference is a rank just above the
// player — that is what makes the ladder a ladder — falling back to below
// them, then to anyone at all.
func findOpponent(ctx context.Context, q queryer, p *Player) (*Player, error) {
	lo := p.Rank - game.LadderBandAbove
	if lo < 1 {
		lo = 1
	}
	queries := []struct {
		sql  string
		args []any
	}{
		{`SELECT ` + playerCols + ` FROM players WHERE id != ? AND ladder_rank >= ? AND ladder_rank < ?
		  ORDER BY RANDOM() LIMIT 1`, []any{p.ID, lo, p.Rank}},
		{`SELECT ` + playerCols + ` FROM players WHERE id != ? AND ladder_rank > ? AND ladder_rank <= ?
		  ORDER BY RANDOM() LIMIT 1`, []any{p.ID, p.Rank, p.Rank + game.LadderBandAbove}},
		{`SELECT ` + playerCols + ` FROM players WHERE id != ? ORDER BY RANDOM() LIMIT 1`, []any{p.ID}},
	}
	for _, qq := range queries {
		op, err := scanPlayer(q.QueryRowContext(ctx, qq.sql, qq.args...))
		if err == nil {
			return op, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return nil, ErrNoOpponent
}

func opponentView(ctx context.Context, q queryer, op *Player) (*Opponent, game.Build, error) {
	build, items, err := buildFor(ctx, q, op)
	if err != nil {
		return nil, game.Build{}, err
	}
	notable := make([]string, 0, 3)
	for _, it := range items {
		if game.RarityOrder[it.Rarity] >= game.RarityOrder[game.Epic] {
			notable = append(notable, it.Name)
		}
	}
	ch, _ := game.ChassisByID(op.ChassisID)
	return &Opponent{
		PlayerID: op.ID, Name: op.DisplayName, Rank: op.Rank, Level: op.Level,
		IsBot: op.IsBot, ChassisID: op.ChassisID, Chassis: ch.Name, LiveryID: op.LiveryID,
		Rating: round2(game.OverallRating(build)), Build: build, Items: notable,
	}, build, nil
}

// NextOpponent shows who is up next without spending a race attempt.
func (s *Store) NextOpponent(ctx context.Context, p *Player) (*Opponent, error) {
	op, err := findOpponent(ctx, s.db, p)
	if err != nil {
		return nil, err
	}
	view, _, err := opponentView(ctx, s.db, op)
	return view, err
}

// RaceOutcome is the full server-authored record of one race.
type RaceOutcome struct {
	RaceID     string          `json:"race_id"`
	Opponent   *Opponent       `json:"opponent"`
	Replay     game.RaceResult `json:"replay"`
	Won        bool            `json:"won"`
	Reward     game.RaceReward `json:"reward"`
	RankBefore int             `json:"rank_before"`
	RankAfter  int             `json:"rank_after"`
	Drop       *InventoryItem  `json:"drop,omitempty"`
	LeveledTo  int             `json:"leveled_to,omitempty"`
	NewPoints  int             `json:"stat_points_awarded,omitempty"`
	SpentRuby  int64           `json:"rubies_spent,omitempty"`
	FreeRaces  int             `json:"free_races_left"`
	Player     *Player         `json:"player"`
	YourRating float64         `json:"your_rating"`
}

// RunRace is the whole race transaction: spend an attempt, pick an opponent,
// simulate, pay out, move the ladder, roll a drop. The client sends no inputs
// beyond "go" and receives a replay it can only render, not influence.
func (s *Store) RunRace(ctx context.Context, playerID string) (*RaceOutcome, error) {
	var out *RaceOutcome
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if err := applyDailyReset(ctx, tx, p, now); err != nil {
			return err
		}

		// --- pay for the attempt ---
		var spentRuby int64
		switch {
		case p.FreeRaces > 0:
			if err := execGuarded(ctx, tx, ErrNoRaces,
				`UPDATE players SET free_races = free_races - 1 WHERE id = ? AND free_races > 0`,
				playerID); err != nil {
				return err
			}
			p.FreeRaces--
		case p.ExtraUsed >= game.MaxExtraRaces:
			return ErrNoRaces
		case p.Rubies >= game.ExtraRaceRubies:
			if err := execGuarded(ctx, tx, ErrNoRaces,
				`UPDATE players SET rubies = rubies - ?, extra_races_used = extra_races_used + 1
				 WHERE id = ? AND rubies >= ?`,
				game.ExtraRaceRubies, playerID, game.ExtraRaceRubies); err != nil {
				return err
			}
			spentRuby = game.ExtraRaceRubies
			p.Rubies -= spentRuby
			p.ExtraUsed++
		default:
			return ErrNoRaces
		}

		// --- assemble the grid ---
		opRow, err := findOpponent(ctx, tx, p)
		if err != nil {
			return err
		}
		opView, opBuild, err := opponentView(ctx, tx, opRow)
		if err != nil {
			return err
		}
		myBuild, _, err := buildFor(ctx, tx, p)
		if err != nil {
			return err
		}

		// --- simulate ---
		seed := randomUint64()
		track := game.PickTrack(seed)
		result := game.SimulateRace(track, myBuild, opBuild, seed)

		reward := game.RaceRewardFor(result.ChallWon, opRow.Rank, p.Level)
		level, xp, points := game.ApplyXP(p.Level, p.XP, reward.XP)

		rankBefore := p.Rank
		rankAfter := p.Rank

		// --- ladder movement: win against someone above you and take their slot ---
		if result.ChallWon && opRow.Rank < p.Rank {
			if _, err := tx.ExecContext(ctx,
				`UPDATE players SET ladder_rank = ladder_rank + 1
				 WHERE ladder_rank >= ? AND ladder_rank < ?`, opRow.Rank, p.Rank); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE players SET ladder_rank = ? WHERE id = ?`, opRow.Rank, playerID); err != nil {
				return err
			}
			rankAfter = opRow.Rank
		}

		winInc, lossInc := 0, 1
		if result.ChallWon {
			winInc, lossInc = 1, 0
		}
		if _, err := tx.ExecContext(ctx, `UPDATE players
			SET cash = cash + ?, driver_level = ?, driver_xp = ?, unspent_points = unspent_points + ?,
			    wins = wins + ?, losses = losses + ?, updated_at = ?
			WHERE id = ?`,
			reward.Cash, level, xp, points, winInc, lossInc, now.Unix(), playerID); err != nil {
			return err
		}

		// --- drops ---
		var drop *InventoryItem
		dropChance := 0.05
		if result.ChallWon {
			dropChance = 0.13
		}
		// Quality tracks how strong the opponent was: beating the top of the
		// mountain is how the named items actually enter circulation.
		quality := clamp01((opView.Rating - 95) / 110)
		rng := rand.New(rand.NewPCG(seed^0xD1B54A32D192ED03, randomUint64()))
		if d, ok := game.RollDrop(rng, dropChance, quality); ok {
			drop, err = insertItem(ctx, tx, playerID, d, "race", now)
			if err != nil {
				return err
			}
		}

		// --- persist the replay so it can be re-derived and audited ---
		replayJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		raceID := NewID()
		var dropID any
		if drop != nil {
			dropID = drop.ItemID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO races
			(id, challenger_id, opponent_id, opponent_name, track_id, seed, challenger_won,
			 margin, cash_awarded, xp_awarded, rank_before, rank_after, drop_item_id, replay, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			raceID, playerID, opRow.ID, opRow.DisplayName, track.ID, int64(seed), boolInt(result.ChallWon),
			result.Margin, reward.Cash, reward.XP, rankBefore, rankAfter, dropID,
			string(replayJSON), now.Unix()); err != nil {
			return err
		}

		after, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		out = &RaceOutcome{
			RaceID: raceID, Opponent: opView, Replay: result, Won: result.ChallWon,
			Reward: reward, RankBefore: rankBefore, RankAfter: rankAfter, Drop: drop,
			SpentRuby: spentRuby, FreeRaces: after.FreeRaces, Player: after,
			YourRating: round2(game.OverallRating(myBuild)),
		}
		if level > p.Level {
			out.LeveledTo = level
			out.NewPoints = points
		}
		return nil
	})
	return out, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Ladder
// ---------------------------------------------------------------------------

type LadderEntry struct {
	Rank     int     `json:"rank"`
	PlayerID string  `json:"player_id"`
	Name     string  `json:"name"`
	Level    int     `json:"driver_level"`
	IsBot    bool    `json:"is_bot"`
	Chassis  string  `json:"chassis_name"`
	LiveryID string  `json:"livery_id"`
	Rating   float64 `json:"rating"`
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	IsYou    bool    `json:"is_you"`
}

// Ladder returns a window of the standings with every rating recomputed from
// the same model the race uses, so what you see is what you will face.
func (s *Store) Ladder(ctx context.Context, viewerID string, from, limit int) ([]LadderEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if from < 1 {
		from = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+playerCols+` FROM players
		WHERE ladder_rank >= ? ORDER BY ladder_rank ASC LIMIT ?`, from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []*Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.entriesFor(ctx, viewerID, players)
}

// LadderAround centres the window on the viewer.
func (s *Store) LadderAround(ctx context.Context, p *Player, span int) ([]LadderEntry, error) {
	from := p.Rank - span/2
	if from < 1 {
		from = 1
	}
	return s.Ladder(ctx, p.ID, from, span)
}

// entriesFor batches the equipment lookup so rendering a ladder page is two
// queries rather than one per row.
func (s *Store) entriesFor(ctx context.Context, viewerID string, players []*Player) ([]LadderEntry, error) {
	if len(players) == 0 {
		return []LadderEntry{}, nil
	}
	ids := make([]any, 0, len(players))
	for _, p := range players {
		ids = append(ids, p.ID)
	}
	q := `SELECT player_id, item_id, quality FROM inventory WHERE equipped = 1 AND player_id IN (?` +
		strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := s.db.QueryContext(ctx, q, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bonuses := map[string][]game.EquipBonus{}
	for rows.Next() {
		var pid, itemID string
		var quality float64
		if err := rows.Scan(&pid, &itemID, &quality); err != nil {
			return nil, err
		}
		if def, ok := game.ItemByID(itemID); ok {
			bonuses[pid] = append(bonuses[pid], game.ItemBonus(def, quality))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]LadderEntry, 0, len(players))
	for _, p := range players {
		total := game.SumBonuses(bonuses[p.ID]...)
		b := game.Build{
			Car:    game.ComputeCar(p.ChassisID, p.Tiers, total),
			Driver: p.Stats.Add(total.Driver),
		}
		ch, _ := game.ChassisByID(p.ChassisID)
		out = append(out, LadderEntry{
			Rank: p.Rank, PlayerID: p.ID, Name: p.DisplayName, Level: p.Level,
			IsBot: p.IsBot, Chassis: ch.Name, LiveryID: p.LiveryID,
			Rating: round2(game.OverallRating(b)), Wins: p.Wins, Losses: p.Losses,
			IsYou: p.ID == viewerID,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// History
// ---------------------------------------------------------------------------

type RaceSummary struct {
	RaceID     string    `json:"race_id"`
	Opponent   string    `json:"opponent"`
	TrackID    string    `json:"track_id"`
	TrackName  string    `json:"track_name"`
	Won        bool      `json:"won"`
	Margin     float64   `json:"margin"`
	Cash       int64     `json:"cash"`
	XP         int64     `json:"xp"`
	RankBefore int       `json:"rank_before"`
	RankAfter  int       `json:"rank_after"`
	DropItem   string    `json:"drop_item,omitempty"`
	At         time.Time `json:"at"`
}

func (s *Store) RaceHistory(ctx context.Context, playerID string, limit int) ([]RaceSummary, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, opponent_name, track_id, challenger_won,
		margin, cash_awarded, xp_awarded, rank_before, rank_after, drop_item_id, created_at
		FROM races WHERE challenger_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RaceSummary{}
	for rows.Next() {
		var r RaceSummary
		var won int
		var dropID sql.NullString
		var at int64
		if err := rows.Scan(&r.RaceID, &r.Opponent, &r.TrackID, &won, &r.Margin, &r.Cash,
			&r.XP, &r.RankBefore, &r.RankAfter, &dropID, &at); err != nil {
			return nil, err
		}
		r.Won = won != 0
		r.At = time.Unix(at, 0).UTC()
		if t, ok := game.TrackByID(r.TrackID); ok {
			r.TrackName = t.Name
		}
		if dropID.Valid {
			if def, ok := game.ItemByID(dropID.String); ok {
				r.DropItem = def.Name
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Replay returns the stored corner-by-corner record of a past race.
func (s *Store) Replay(ctx context.Context, playerID, raceID string) (*game.RaceResult, error) {
	var blob string
	err := s.db.QueryRowContext(ctx, `SELECT replay FROM races WHERE id = ? AND challenger_id = ?`,
		raceID, playerID).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var r game.RaceResult
	if err := json.Unmarshal([]byte(blob), &r); err != nil {
		return nil, err
	}
	return &r, nil
}
