package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
)

func TestPersistenceAndStaleRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graphs.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	g := domain.Seeds()[0]
	ctx := context.Background()
	if e = s.Save(ctx, &g); e != nil {
		t.Fatal(e)
	}
	stale := g
	g.Title = "Changed"
	if e = s.Save(ctx, &g); e != nil {
		t.Fatal(e)
	}
	if e = s.Save(ctx, &stale); !errors.Is(e, domain.ErrConflict) {
		t.Fatalf("stale save: %v", e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got, e := s.Get(ctx, g.ID)
	if e != nil || got.Title != "Changed" || got.Revision != 2 {
		t.Fatalf("reopen: %#v %v", got, e)
	}
}
