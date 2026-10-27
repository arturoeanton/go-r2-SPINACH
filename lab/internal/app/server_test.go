package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/storage"
)

type decisions struct {
	calls int
	model string
	err   error
}

func (d *decisions) Rank(_ context.Context, _ domain.Graph, _ string, model string) (models.Decision, error) {
	d.calls++
	d.model = model
	return models.Decision{Model: "test-jev", Choice: "geografia", Source: "test", Candidates: []models.Candidate{}}, d.err
}

type languageModel struct {
	action models.Action
	focus  string
	model  string
	err    error
}

func (l *languageModel) Next(_ context.Context, _ domain.Graph, _, model, focus, _ string, _ []models.Event) (models.Action, error) {
	l.focus = focus
	l.model = model
	return l.action, l.err
}

func TestNavigationAndTimingSwitches(t *testing.T) {
	for _, tc := range []struct {
		name       string
		navigation *bool
		measure    bool
	}{
		{"defaults", nil, false},
		{"Jev timed", boolValue(true), true},
		{"no Jev timed", boolValue(false), true},
		{"no Jev untimed", boolValue(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			d := &decisions{}
			l := &languageModel{action: models.Action{Type: "query", Query: domain.Seeds()[0].Examples[0].Query}}
			s.Decisions, s.Language = d, l
			useNavigation := tc.navigation == nil || *tc.navigation
			if !useNavigation {
				d.err = errors.New("Jev unavailable; this must not block OpenAI")
			}
			b := request{Question: "Who?", UseNavigation: tc.navigation, MeasureTime: tc.measure, Focus: "stale-subgraph"}
			w := post(s, "/api/graphs/aurora/step", b)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var event models.Event
			if err := json.Unmarshal(w.Body.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if (event.Navigation != nil) != useNavigation || d.calls != boolInt(useNavigation) {
				t.Fatal("unexpected navigation calls", d.calls, w.Body.String())
			}
			if useNavigation && l.focus != "geografia" || !useNavigation && l.focus != "" {
				t.Fatal("stale focus leaked into direct exploration", l.focus)
			}
			if (event.Timing != nil) != tc.measure {
				t.Fatal("timing switch ignored", w.Body.String())
			}
			if event.Timing != nil {
				m := event.Timing
				if m.GenerationMS < 0 || m.ToolMS < 0 || m.TotalMS <= 0 || m.TotalMS < m.NavigationMS+m.GenerationMS+m.ReplayMS+m.ToolMS {
					t.Fatal("invalid timing breakdown", m)
				}
				if !useNavigation && m.NavigationMS != 0 {
					t.Fatal("Jev work reported when disabled", m)
				}
			}
			// Turning navigation off cannot bypass the evidence requirement.
			l.action = models.Action{Type: "answer", Answer: "unsupported"}
			w = post(s, "/api/graphs/aurora/step", b)
			if !bytes.Contains(w.Body.Bytes(), []byte("Antes de responder")) {
				t.Fatal("premature answer accepted", w.Body.String())
			}
		})
	}
}

func TestDisabledRouteAndTimedFailures(t *testing.T) {
	s := testServer(t)
	d := &decisions{err: errors.New("navigation unavailable")}
	l := &languageModel{err: errors.New("generation unavailable")}
	s.Decisions, s.Language = d, l
	b := request{Question: "Who?", UseNavigation: boolValue(false), MeasureTime: true}
	w := post(s, "/api/graphs/aurora/route", b)
	if w.Code != 200 || d.calls != 0 || !bytes.Contains(w.Body.Bytes(), []byte(`"source":"disabled"`)) {
		t.Fatal("disabled route called Jev", w.Code, w.Body.String())
	}
	for _, useNavigation := range []bool{false, true} {
		b.UseNavigation = boolValue(useNavigation)
		w = post(s, "/api/graphs/aurora/step", b)
		var failure struct {
			Error  string
			Timing *models.Timing
		}
		if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
			t.Fatal(err)
		}
		if w.Code != 502 || failure.Timing == nil || failure.Timing.TotalMS <= 0 {
			t.Fatal("failed request lost timing", w.Code, w.Body.String())
		}
	}
}

func boolValue(v bool) *bool { return &v }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
	return New(repo, models.New(), sources)
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
func TestJevChoosesEveryStepAndEvidenceIsRebuilt(t *testing.T) {
	s := testServer(t)
	d := &decisions{}
	l := &languageModel{action: models.Action{Type: "query", Query: domain.Seeds()[0].Examples[0].Query, Objective: "query evidence"}}
	s.Decisions = d
	s.Language = l
	w := post(s, "/api/graphs/aurora/step", request{Question: "Who?", Language: "en"})
	if w.Code != 200 || d.calls != 1 || l.focus != "geografia" {
		t.Fatalf("%d %s calls=%d focus=%s", w.Code, w.Body.String(), d.calls, l.focus)
	}
	l.action = models.Action{Type: "answer", Answer: "test"}
	w = post(s, "/api/graphs/aurora/step", request{Question: "Who?", Events: []models.Event{{Action: models.Action{Type: "search", Text: "Nobody"}, Observation: map[string]any{"rows": []any{"fabricated"}}}}})
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

func TestServerOwnsModelConfiguration(t *testing.T) {
	s := testServer(t)
	s.Models.DefaultModel = "server-generation-model"
	s.Models.DefaultNavigationModel = "server-navigation-model"
	s.Models.OpenAIKey, s.Models.JevToken = "private-openai-test", "private-jev-test"
	d := &decisions{}
	l := &languageModel{action: models.Action{Type: "query", Query: domain.Seeds()[0].Examples[0].Query}}
	s.Decisions, s.Language = d, l
	for _, endpoint := range []string{"route", "step"} {
		path := "/api/graphs/aurora/" + endpoint
		w := post(s, path, request{Question: "Who?"})
		if w.Code != 200 || d.model != s.Models.DefaultNavigationModel {
			t.Fatal("server navigation model was not used", w.Code, w.Body.String())
		}
		if endpoint == "step" && (l.model != s.Models.DefaultModel || !bytes.Contains(w.Body.Bytes(), []byte(`"model":"server-generation-model"`))) {
			t.Fatal("server generation model was not used in action and trace", w.Body.String())
		}
		for _, field := range []string{"model", "navigationModel"} {
			w = post(s, path, map[string]any{"question": "Who?", field: "browser-override"})
			if w.Code != 400 {
				t.Fatal("browser model override accepted", field, w.Code)
			}
		}
	}
	for _, endpoint := range []string{"/api/credentials", "/api/config"} {
		w := post(s, endpoint, map[string]string{"openaiApiKey": "replacement-test-key", "model": "override"})
		if w.Code < 400 || s.Models.OpenAIKey != "private-openai-test" || s.Models.DefaultModel != "server-generation-model" {
			t.Fatal("web configuration write accepted", endpoint, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/config", nil))
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(s.Models.OpenAIKey)) || bytes.Contains(w.Body.Bytes(), []byte(s.Models.JevToken)) {
		t.Fatal("public configuration leaked a key", w.Code)
	}
}
