package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/storage"
)

type decisions struct{ calls int }

func (d *decisions) Rank(_ context.Context, _ domain.Graph, _ string, _ string) (models.Decision, error) {
	d.calls++
	return models.Decision{Model: "test-laya", Choice: "geografia", Source: "test", Candidates: []models.Candidate{}}, nil
}

type languageModel struct {
	action models.Action
	focus  string
}

func (l *languageModel) Next(_ context.Context, _ domain.Graph, _, _, focus, _ string, _ []models.Event) (models.Action, error) {
	l.focus = focus
	return l.action, nil
}
func testServer(t *testing.T) *Server {
	t.Helper()
	repo, e := storage.Open(filepath.Join(t.TempDir(), "lab.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { repo.Close() })
	g := domain.Seeds()[0]
	if e = repo.Save(context.Background(), &g); e != nil {
		t.Fatal(e)
	}
	sources, e := storage.NewSources(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { sources.Close() })
	return New(repo, models.New("http://unused", "http://unused"), sources)
}
func post(s *Server, path string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestRealQueryAndValidation(t *testing.T) {
	s := testServer(t)
	g := domain.Seeds()[0]
	w := post(s, "/api/graphs/aurora/query", map[string]string{"query": g.Examples[0].Query})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var res struct {
		Rows []map[string]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &res); e != nil {
		t.Fatal(e)
	}
	if len(res.Rows) != 1 || res.Rows[0]["persona"].Value != domain.Namespace+"elena" {
		t.Fatal(w.Body.String())
	}
	w = post(s, "/api/import", map[string]string{"format": "json", "content": `{"title":"Broken","nodes":[],"edges":[{"id":"x","source":"missing","target":"missing","predicate":"urn:p"}]}`})
	if w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestLayaChoosesEveryStepAndEvidenceIsRebuilt(t *testing.T) {
	s := testServer(t)
	d := &decisions{}
	l := &languageModel{action: models.Action{Type: "query", Query: domain.Seeds()[0].Examples[0].Query, Objective: "query evidence"}}
	s.Decisions = d
	s.Language = l
	w := post(s, "/api/graphs/aurora/step", request{Question: "Who?", Model: "gemma", NavigationModel: "laya", Language: "en"})
	if w.Code != 200 || d.calls != 1 || l.focus != "geografia" {
		t.Fatalf("%d %s calls=%d focus=%s", w.Code, w.Body.String(), d.calls, l.focus)
	}
	l.action = models.Action{Type: "answer", Answer: "test"}
	w = post(s, "/api/graphs/aurora/step", request{Question: "Who?", Model: "gemma", Events: []models.Event{{Action: models.Action{Type: "search", Text: "Nobody"}, Observation: map[string]any{"rows": []any{"fabricated"}}}}})
	if !bytes.Contains(w.Body.Bytes(), []byte("Antes de responder")) {
		t.Fatal("client evidence was trusted:", w.Body.String())
	}
}
func TestCrossOriginWritesRejected(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest("POST", "/api/graphs", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://unrelated.example")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
