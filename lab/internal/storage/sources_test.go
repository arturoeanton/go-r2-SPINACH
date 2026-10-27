package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceDeletionClosesCopyAndPreservesOriginal(t *testing.T) {
	ctx := context.Background()
	original := filepath.Join(t.TempDir(), "original.sqlite")
	db, err := sql.Open("sqlite", original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE people(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO people VALUES(1, 'Ana')"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSources(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	id := "0123456789abcdef01234567"
	if _, err = s.Register(ctx, id, data); err != nil {
		t.Fatal(err)
	}
	src, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.Dir, id+".sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copy remains: %v", err)
	}
	if _, err = s.Open(id); err == nil {
		t.Fatal("deleted source reopened")
	}
	if _, _, err = src.Query(ctx, "SELECT ?s ?p ?o WHERE {?s ?p ?o}"); err == nil {
		t.Fatal("cached handle still queries deleted database")
	}
	if err = s.Delete(id); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatalf("original removed: %v", err)
	}
	for _, invalid := range []string{"", "../original", id + "/../original", "zz23456789abcdef01234567"} {
		if err = s.Delete(invalid); err == nil {
			t.Fatalf("accepted unsafe identifier %q", invalid)
		}
	}
	if _, err = s.Register(ctx, id, data); err != nil {
		t.Fatalf("reimport failed: %v", err)
	}
}
