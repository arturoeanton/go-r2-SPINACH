package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/storage"
)

func uploadedDatabase(t *testing.T, s *Server) domain.Graph {
	t.Helper()
	path := filepath.Join(t.TempDir(), "original.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE people(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO people VALUES(1, 'Ana')"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	w := post(s, "/api/databases", map[string]any{"title": "Test upload", "content": data})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var g domain.Graph
	if err = json.Unmarshal(w.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func deleteGraph(s *Server, id string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("DELETE", "/api/graphs/"+id, bytes.NewBufferString("{}"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestDeleteUploadedDatabase(t *testing.T) {
	s := testServer(t)
	g := uploadedDatabase(t, s)
	other := uploadedDatabase(t, s)
	ctx := context.Background()
	w := deleteGraph(s, g.ID)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"sourceDeleted":true`)) {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.Repo.Get(ctx, g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("graph remains: %v", err)
	}
	registry := s.Sources.(*storage.Sources)
	if _, err := os.Stat(filepath.Join(registry.Dir, g.SQLiteSource+".sqlite")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copy remains: %v", err)
	}
	if _, err := s.Sources.Open(g.SQLiteSource); err == nil {
		t.Fatal("deleted source still accessible")
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/databases/"+g.SQLiteSource+"/schema", nil))
	if w.Code != 404 {
		t.Fatal("deleted schema accessible", w.Code)
	}
	if w = post(s, "/api/graphs/"+other.ID+"/query", map[string]string{"query": "SELECT ?s ?p ?o WHERE {?s ?p ?o}"}); w.Code != 200 {
		t.Fatal("other upload broken", w.Body.String())
	}
	if _, err := s.Repo.Get(ctx, "aurora"); err != nil {
		t.Fatal("seed removed", err)
	}
	if w = deleteGraph(s, g.ID); w.Code != 404 {
		t.Fatal("repeat delete", w.Code)
	}
	// A stale exported graph cannot recreate a reference to the removed file.
	if w = post(s, "/api/graphs", g); w.Code != 422 {
		t.Fatal("missing source accepted", w.Code, w.Body.String())
	}
}

func TestSharedDatabaseDeletedAfterLastUniverse(t *testing.T) {
	s := testServer(t)
	g := uploadedDatabase(t, s)
	w := post(s, "/api/graphs", g)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var copy domain.Graph
	if err := json.Unmarshal(w.Body.Bytes(), &copy); err != nil {
		t.Fatal(err)
	}
	w = deleteGraph(s, g.ID)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"sourceDeleted":false`)) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = post(s, "/api/graphs/"+copy.ID+"/query", map[string]string{"query": "SELECT ?s ?p ?o WHERE {?s ?p ?o}"}); w.Code != 200 {
		t.Fatal("shared source removed", w.Body.String())
	}
	if w = deleteGraph(s, copy.ID); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.Sources.Open(g.SQLiteSource); err == nil {
		t.Fatal("last reference did not remove file")
	}
}

type failingSourceDelete struct{ SourceRegistry }

func (f failingSourceDelete) Delete(string) error { return errors.New("disk removal failed") }

func TestFailedDatabaseRemovalCanBeRetried(t *testing.T) {
	s := testServer(t)
	g := uploadedDatabase(t, s)
	registry := s.Sources
	s.Sources = failingSourceDelete{registry}
	w := deleteGraph(s, g.ID)
	if w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.Repo.Get(context.Background(), g.ID); err != nil {
		t.Fatal("graph lost after failed cleanup", err)
	}
	if _, err := registry.Open(g.SQLiteSource); err != nil {
		t.Fatal("source lost after failed cleanup", err)
	}
	s.Sources = registry
	if w = deleteGraph(s, g.ID); w.Code != 200 {
		t.Fatal("retry failed", w.Code, w.Body.String())
	}
}

func TestFailedDatabaseImportDoesNotLeaveCopy(t *testing.T) {
	s := testServer(t)
	g := uploadedDatabase(t, s)
	registry := s.Sources.(*storage.Sources)
	data, err := os.ReadFile(filepath.Join(registry.Dir, g.SQLiteSource+".sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	w := post(s, "/api/databases", map[string]any{"title": strings.Repeat("x", 161), "content": data})
	if w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
	files, err := os.ReadDir(registry.Dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("orphaned copy after failed import: %v %v", files, err)
	}
}
