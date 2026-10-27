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
func TestDirectExplorationHasGraphConnectionsWithoutStaleFocus(t *testing.T) {
	services := New()
	services.OpenAIKey = "test-openai"
	services.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var req struct {
			Input string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		var state struct {
			ActiveSubgraph  string
			InitialSubgraph []domain.Edge
		}
		if err := json.Unmarshal([]byte(req.Input), &state); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(req.Input, "\"examples\"") {
			t.Fatal("solved example queries leaked to the generator")
		}
		if state.ActiveSubgraph != "" || len(state.InitialSubgraph) != len(domain.Seeds()[0].Edges) {
			t.Fatal("direct exploration needs an unfiltered, bounded graph preview", state)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"completed","model":"gpt-5","output":[{"type":"function_call","name":"search","arguments":"{\"objective\":\"Find Elena\",\"text\":\"Elena\"}"}]}`))}, nil
	})}
	if _, err := services.Next(context.Background(), domain.Seeds()[0], "Who?", "gpt-5", "", "en", nil); err != nil {
		t.Fatal(err)
	}
}

func TestModelReceivesActualTypedLiteralValues(t *testing.T) {
	services := New()
	services.OpenAIKey = "test-openai"
	services.Client = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var request struct{ Input string }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var state struct {
			InitialSubgraph []struct {
				Target string
				Object struct{ Kind, Value, Datatype string }
			}
		}
		if err := json.Unmarshal([]byte(request.Input), &state); err != nil {
			t.Fatal(err)
		}
		literals := 0
		for _, edge := range state.InitialSubgraph {
			if edge.Object.Kind == "literal" {
				literals++
				if edge.Object.Value == edge.Target || edge.Object.Datatype != "http://www.w3.org/2001/XMLSchema#integer" {
					t.Fatal("literal identifiers substituted for RDF values")
				}
			}
		}
		if literals == 0 {
			t.Fatal("fixture did not expose any numeric values")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"completed","model":"gpt-5","output":[{"type":"function_call","name":"search","arguments":"{\"objective\":\"Inspect the data\",\"text\":\"Paper\"}"}]}`))}, nil
	})}
	for _, focus := range []string{"", "publications"} {
		if _, err := services.Next(context.Background(), domain.ChallengeGraph(), "Sum citations", "gpt-5", focus, "en", nil); err != nil {
			t.Fatal(err)
		}
	}
}
