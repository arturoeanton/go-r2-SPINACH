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

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLMStudioDoesNotOfferAnswerWithoutEvidence(t *testing.T) {
	local := New("http://unused", "http://unused")
	local.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatal(r.URL)
		}
		var req map[string]any
		if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
			t.Fatal(e)
		}
		schema := req["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
		for _, v := range schema["oneOf"].([]any) {
			kind := v.(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)["const"]
			if kind == "answer" {
				t.Fatal("answer offered before evidence")
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"type\":\"query\",\"objective\":\"Get evidence\",\"query\":\"SELECT * WHERE {?s ?p ?o} LIMIT 5\"}"}}]}`)), Header: make(http.Header)}, nil
	})}
	a, e := local.Next(context.Background(), domain.Seeds()[0], "Who?", "google/gemma-4-12b", "ciencia", "en", nil)
	if e != nil || a.Type != "query" {
		t.Fatal(a, e)
	}
}
