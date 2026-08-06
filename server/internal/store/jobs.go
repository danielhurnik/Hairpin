package store

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

type ActiveJob struct {
	ID          int64     `json:"id"`
	JobID       string    `json:"job_id"`
	Name        string    `json:"name"`
	Tier        int       `json:"tier"`
	StartedAt   time.Time `json:"started_at"`
	CompletesAt time.Time `json:"completes_at"`
	RemainingS  int64     `json:"remaining_seconds"`
	Ready       bool      `json:"ready"`
	RushRubies  int64     `json:"rush_rubies"`
	Cash        int64     `json:"cash"`
	XP          int64     `json:"xp"`
}

// activeJob reads the player's unclaimed job, if any.
func activeJob(ctx context.Context, q queryer, playerID string, now time.Time) (*ActiveJob, error) {
	var a ActiveJob
	var started, completes int64
	err := q.QueryRowContext(ctx, `SELECT id, job_id, started_at, completes_at
		FROM player_jobs WHERE player_id = ? AND claimed_at IS NULL`, playerID).
		Scan(&a.ID, &a.JobID, &started, &completes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.StartedAt = time.Unix(started, 0).UTC()
	a.CompletesAt = time.Unix(completes, 0).UTC()
	remaining := a.CompletesAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	a.RemainingS = int64(remaining.Seconds())
	a.Ready = remaining <= 0
	a.RushRubies = game.RushCost(remaining)
	if def, ok := game.JobByID(a.JobID); ok {
		a.Name = def.Name
		a.Tier = def.Tier
		a.Cash = def.Cash
		a.XP = def.XP
	}
	return &a, nil
}

func (s *Store) ActiveJob(ctx context.Context, playerID string) (*ActiveJob, error) {
	return activeJob(ctx, s.db, playerID, s.Now())
}

// StartJob begins a timed job. The completion time is computed and stored by
// the server; the client is only ever told when it will be done.
func (s *Store) StartJob(ctx context.Context, playerID, jobID string) (*ActiveJob, error) {
	def, ok := game.JobByID(jobID)
	if !ok {
		return nil, ErrInvalid
	}
	var out *ActiveJob
	err := s.tx(ctx, func(tx *sql.Tx) error {
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if p.Level < def.MinLevel {
			return ErrInvalid
		}
		existing, err := activeJob(ctx, tx, playerID, s.Now())
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrJobActive
		}

		now := s.Now()
		dur := time.Duration(float64(def.Duration) / s.timeScale)
		if dur < time.Second {
			dur = time.Second
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO player_jobs
			(player_id, job_id, started_at, completes_at) VALUES (?, ?, ?, ?)`,
			playerID, def.ID, now.Unix(), now.Add(dur).Unix())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		_ = id
		if err := touch(ctx, tx, playerID, now); err != nil {
			return err
		}
		out, err = activeJob(ctx, tx, playerID, now)
		return err
	})
	return out, err
}

// ClaimResult is what the player walks away with.
type ClaimResult struct {
	Job       string         `json:"job"`
	Cash      int64          `json:"cash"`
	XP        int64          `json:"xp"`
	LeveledTo int            `json:"leveled_to,omitempty"`
	NewPoints int            `json:"stat_points_awarded,omitempty"`
	Drop      *InventoryItem `json:"drop,omitempty"`
	TotalCash int64          `json:"total_cash"`
	DriverLvl int            `json:"driver_level"`
	DriverXP  int64          `json:"driver_xp"`
	XPToNext  int64          `json:"driver_xp_to_next"`
}

// ClaimJob pays out a finished job. The timer is re-checked against the
// server clock here — a client that lies about completion gets nothing.
func (s *Store) ClaimJob(ctx context.Context, playerID string) (*ClaimResult, error) {
	var out *ClaimResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		job, err := activeJob(ctx, tx, playerID, now)
		if err != nil {
			return err
		}
		if job == nil {
			return ErrNoJob
		}
		if !job.Ready {
			return ErrJobNotDone
		}
		def, ok := game.JobByID(job.JobID)
		if !ok {
			return ErrInvalid
		}
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}

		// Mark claimed first: the unique partial index means a concurrent
		// second claim finds no active job and gets ErrNoJob.
		res, err := tx.ExecContext(ctx,
			`UPDATE player_jobs SET claimed_at = ? WHERE id = ? AND claimed_at IS NULL`,
			now.Unix(), job.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNoJob
		}

		level, xp, points := game.ApplyXP(p.Level, p.XP, def.XP)
		result := &ClaimResult{Job: def.Name, Cash: def.Cash, XP: def.XP}
		if level > p.Level {
			result.LeveledTo = level
			result.NewPoints = points
		}

		if _, err := tx.ExecContext(ctx, `UPDATE players
			SET cash = cash + ?, driver_level = ?, driver_xp = ?, unspent_points = unspent_points + ?, updated_at = ?
			WHERE id = ?`, def.Cash, level, xp, points, now.Unix(), playerID); err != nil {
			return err
		}

		rng := rand.New(rand.NewPCG(randomUint64(), randomUint64()))
		if drop, ok := game.RollDrop(rng, def.DropChance, def.DropQuality); ok {
			item, err := insertItem(ctx, tx, playerID, drop, "job", now)
			if err != nil {
				return err
			}
			result.Drop = item
		}

		after, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		result.TotalCash = after.Cash
		result.DriverLvl = after.Level
		result.DriverXP = after.XP
		result.XPToNext = after.XPToNext
		out = result
		return nil
	})
	return out, err
}

// RushJob spends rubies to finish a job immediately. The price is computed
// from the server's own remaining-time figure.
func (s *Store) RushJob(ctx context.Context, playerID string) (*ActiveJob, int64, error) {
	var out *ActiveJob
	var spent int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		job, err := activeJob(ctx, tx, playerID, now)
		if err != nil {
			return err
		}
		if job == nil {
			return ErrNoJob
		}
		if job.Ready {
			return ErrInvalid // nothing to skip
		}
		p, err := getPlayer(ctx, tx, playerID)
		if err != nil {
			return err
		}
		cost := game.RushCost(job.CompletesAt.Sub(now))
		if p.Rubies < cost {
			return ErrInsufficient
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE players SET rubies = rubies - ?, updated_at = ? WHERE id = ?`,
			cost, now.Unix(), playerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE player_jobs SET completes_at = ?, rushed = 1 WHERE id = ?`,
			now.Unix(), job.ID); err != nil {
			return err
		}
		spent = cost
		out, err = activeJob(ctx, tx, playerID, now)
		return err
	})
	return out, spent, err
}

// AvailableJobs describes the job board for a player.
type JobOffer struct {
	game.JobDef
	Locked      bool `json:"locked"`
	UnlocksAt   int  `json:"unlocks_at_level"`
	DurationSec int  `json:"duration_seconds"`
}

func (s *Store) JobBoard(ctx context.Context, p *Player) []JobOffer {
	out := make([]JobOffer, 0, len(game.Jobs))
	for _, j := range game.Jobs {
		scaled := time.Duration(float64(j.Duration) / s.timeScale)
		if scaled < time.Second {
			scaled = time.Second
		}
		j.DurationS = int(scaled.Seconds())
		out = append(out, JobOffer{
			JobDef:      j,
			Locked:      p.Level < j.MinLevel,
			UnlocksAt:   j.MinLevel,
			DurationSec: j.DurationS,
		})
	}
	return out
}
