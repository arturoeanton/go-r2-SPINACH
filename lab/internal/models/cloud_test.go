package models

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
)

func TestOpenAIRealProtocolAndEvidenceGate(t *testing.T) {
	l := New()
	l.OpenAIKey = "test-openai-key"
	withEvidence := false
	l.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.openai.com/v1/responses" || r.Header.Get("Authorization") != "Bearer test-openai-key" {
			t.Fatal("wrong endpoint or auth")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["model"] != "gpt-5" || req["store"] != false || req["parallel_tool_calls"] != false || req["tool_choice"] != "required" || req["temperature"] != nil {
			t.Fatal("invalid GPT-5 request options")
		}
		answerOffered := false
		for _, raw := range req["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["strict"] != true {
				t.Fatal("missing strict schema")
			}
			if tool["name"] == "answer" {
				answerOffered = true
			}
		}
		if answerOffered != withEvidence {
			t.Fatal("answer evidence gate mismatch")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"completed","model":"gpt-5-2025-08-07","output":[{"type":"function_call","name":"query","arguments":"{\"objective\":\"Get evidence\",\"query\":\"SELECT * WHERE {?s ?p ?o} LIMIT 5\"}"}]}`))}, nil
	})}
	for _, events := range [][]Event{nil, {{Action: Action{Type: "query"}}}} {
		a, err := l.Next(context.Background(), domain.Seeds()[0], "Who?", "gpt-5", "", "en", events)
		if err != nil || a.Type != "query" || a.Input["question"] != "Who?" || a.ActualModel != "gpt-5-2025-08-07" {
			t.Fatal(a.Type, err)
		}
		if !strings.Contains(a.Instructions, "supplied function") {
			t.Fatal("trace missed actual instructions")
		}
		withEvidence = true
	}
}
func TestJevUsesOfficialDecisionEndpoint(t *testing.T) {
	l := New()
	l.JevToken = "test-jev-key"
	g := domain.Seeds()[0]
	l.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.typesafe.ai/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-jev-key" {
			t.Fatal("wrong Jev endpoint or auth")
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "jev-latest" {
			t.Fatal("wrong Jev model")
		}
		probabilities := map[string]float64{}
		for _, sub := range g.Subgraphs {
			probabilities[sub.ID] = 1 / float64(len(g.Subgraphs))
		}
		body, _ := json.Marshal(map[string]any{"model": "jev-1.13.0", "answers": map[string]any{"subgraph": map[string]any{"choice": g.Subgraphs[0].ID, "confidence": 0.7, "probabilities": probabilities}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	d, err := l.Rank(context.Background(), g, "Who?", "jev-latest")
	if err != nil || d.Source != "jev" || d.Model != "jev-1.13.0" || d.Input == nil {
		t.Fatal(d, err)
	}
}
func TestCloudErrorsAndPublicConfigDoNotLeakKeys(t *testing.T) {
	l := New()
	l.OpenAIKey = "test-private-token"
	l.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid key test-private-token"}}`))}, nil
	})}
	err := l.call(context.Background(), l.OpenAIURL, "/models", nil, new(any), l.OpenAIKey)
	if err == nil || strings.Contains(err.Error(), l.OpenAIKey) {
		t.Fatal("secret leaked in error")
	}
	config, _ := json.Marshal(l.PublicConfig())
	if strings.Contains(string(config), l.OpenAIKey) {
		t.Fatal("secret leaked in public config")
	}
}

func TestEnvironmentAllowsSetupWithoutKeys(t *testing.T) {
	for _, key := range []string{"OPENAI_API_KEY", "JEV_TOKEN", "JEV_API_KEY", "TYPESAFE_API_KEY", "OPENAI_MODEL", "JEV_MODEL", "OPENAI_BASE_URL", "JEV_BASE_URL", "OPENAI_REASONING_EFFORT"} {
		t.Setenv(key, "")
	}
	services, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	model, navigation := services.Models()
	if model != "gpt-5" || navigation != "jev-latest" {
		t.Fatal("incorrect API defaults")
	}
	if services.LanguageStatus(context.Background()).Online || services.NavigationStatus(context.Background()).Online {
		t.Fatal("missing credentials reported connected")
	}
	t.Setenv("OPENAI_BASE_URL", "https://user:secret@example.com/v1")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("URL-embedded credentials accepted")
	}
}
