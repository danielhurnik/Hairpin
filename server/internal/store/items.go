package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/game"
)

func insertItem(ctx context.Context, q queryer, playerID string, drop game.Drop, source string, now time.Time) (*InventoryItem, error) {
	def, ok := game.ItemByID(drop.ItemID)
	if !ok {
		return nil, ErrInvalid
	}
	res, err := q.ExecContext(ctx, `INSERT INTO inventory
		(player_id, item_id, slot, quality, equipped, source, acquired_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		playerID, def.ID, def.Slot, drop.Quality, source, now.Unix())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &InventoryItem{
		ID: id, ItemID: def.ID, Name: def.Name, Slot: def.Slot, Rarity: def.Rarity,
		Flavour: def.Flavour, Quality: drop.Quality, Source: source,
		SellValue: game.SellValue(def, drop.Quality), Bonus: game.ItemBonus(def, drop.Quality),
		AcquiredAt: now,
	}, nil
}

func (s *Store) Inventory(ctx context.Context, playerID string) ([]InventoryItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, item_id, slot, quality, equipped, source, acquired_at
		FROM inventory WHERE player_id = ? ORDER BY equipped DESC, acquired_at DESC`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInventory(rows)
}

// Equip moves an item into its slot, unequipping whatever was there. Both
// steps happen in one transaction so the "one per slot" index always holds.
func (s *Store) Equip(ctx context.Context, playerID string, itemRowID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var slot string
		var equipped int
		err := tx.QueryRowContext(ctx, `SELECT slot, equipped FROM inventory WHERE id = ? AND player_id = ?`,
			itemRowID, playerID).Scan(&slot, &equipped)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if equipped != 0 {
			return nil // already on
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory SET equipped = 0 WHERE player_id = ? AND slot = ? AND equipped = 1`,
			playerID, slot); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE inventory SET equipped = 1 WHERE id = ?`, itemRowID); err != nil {
			return err
		}
		return touch(ctx, tx, playerID, s.Now())
	})
}

func (s *Store) Unequip(ctx context.Context, playerID string, itemRowID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE inventory SET equipped = 0 WHERE id = ? AND player_id = ?`, itemRowID, playerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Sell converts a spare item back into cash. Equipped items must come off first.
func (s *Store) Sell(ctx context.Context, playerID string, itemRowID int64) (int64, error) {
	var got int64
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var itemID string
		var quality float64
		var equipped int
		err := tx.QueryRowContext(ctx, `SELECT item_id, quality, equipped FROM inventory WHERE id = ? AND player_id = ?`,
			itemRowID, playerID).Scan(&itemID, &quality, &equipped)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if equipped != 0 {
			return ErrInvalid
		}
		def, ok := game.ItemByID(itemID)
		if !ok {
			return ErrInvalid
		}
		value := game.SellValue(def, quality)
		if _, err := tx.ExecContext(ctx, `DELETE FROM inventory WHERE id = ?`, itemRowID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE players SET cash = cash + ?, updated_at = ? WHERE id = ?`,
			value, s.Now().Unix(), playerID); err != nil {
			return err
		}
		got = value
		return nil
	})
	return got, err
}
