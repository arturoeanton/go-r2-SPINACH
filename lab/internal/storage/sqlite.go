package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS graphs (id TEXT PRIMARY KEY, document TEXT NOT NULL, revision INTEGER NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db}, nil
}
func (s *SQLite) Close() error { return s.db.Close() }
func (s *SQLite) List(ctx context.Context) ([]domain.Graph, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT document, revision FROM graphs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Graph{}
	for rows.Next() {
		var raw string
		var g domain.Graph
		if err = rows.Scan(&raw, &g.Revision); err != nil {
			return nil, err
		}
		rev := g.Revision
		if err = json.Unmarshal([]byte(raw), &g); err != nil {
			return nil, err
		}
		g.Revision = rev
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *SQLite) Get(ctx context.Context, id string) (domain.Graph, error) {
	var raw string
	var rev int64
	var g domain.Graph
	err := s.db.QueryRowContext(ctx, "SELECT document, revision FROM graphs WHERE id=?", id).Scan(&raw, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return g, domain.ErrNotFound
	}
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(raw), &g)
	g.Revision = rev
	return g, err
}
func (s *SQLite) Save(ctx context.Context, g *domain.Graph) error {
	if err := g.Validate(); err != nil {
		return err
	}
	next := *g
	next.Revision++
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if g.Revision == 0 {
		_, err = s.db.ExecContext(ctx, "INSERT INTO graphs(id,document,revision) VALUES(?,?,?)", g.ID, string(raw), next.Revision)
		if err != nil {
			var found int
			if s.db.QueryRowContext(ctx, "SELECT 1 FROM graphs WHERE id=?", g.ID).Scan(&found) == nil {
				return domain.ErrConflict
			}
			return err
		}
	} else {
		r, e := s.db.ExecContext(ctx, "UPDATE graphs SET document=?,revision=? WHERE id=? AND revision=?", string(raw), next.Revision, g.ID, g.Revision)
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			return domain.ErrConflict
		}
	}
	g.Revision = next.Revision
	return nil
}
func (s *SQLite) Delete(ctx context.Context, id string) error {
	r, err := s.db.ExecContext(ctx, "DELETE FROM graphs WHERE id=?", id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
