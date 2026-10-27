package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

func environment(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func FromEnvironment() (*Runtime, error) {
	l := New()
	l.OpenAIKey = environment("OPENAI_API_KEY", "")
	l.JevToken = environment("JEV_TOKEN", environment("JEV_API_KEY", environment("TYPESAFE_API_KEY", "")))
	l.OpenAIURL = strings.TrimRight(environment("OPENAI_BASE_URL", l.OpenAIURL), "/")
	l.JevURL = strings.TrimRight(environment("JEV_BASE_URL", l.JevURL), "/")
	l.DefaultModel = environment("OPENAI_MODEL", l.DefaultModel)
	l.DefaultNavigationModel = environment("JEV_MODEL", l.DefaultNavigationModel)
	l.ReasoningEffort = environment("OPENAI_REASONING_EFFORT", "medium")

	switch l.ReasoningEffort {
	case "minimal", "low", "medium", "high":
	default:
		return nil, errors.New("OPENAI_REASONING_EFFORT must be minimal, low, medium or high")
	}
	for _, base := range []string{l.OpenAIURL, l.JevURL} {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid API base URL")
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
			return nil, errors.New("API base URL must use HTTPS")
		}
	}
	return l, nil
}
func (l *Runtime) Models() (string, string) {
	return l.DefaultModel, l.DefaultNavigationModel
}
func (l *Runtime) PublicConfig() map[string]any {
	model, nav := l.Models()
	return map[string]any{"provider": "openai", "navigationProvider": "jev", "generationLabel": "OpenAI", "navigationLabel": "Jev", "defaults": map[string]string{"generation": model, "navigation": nav}}
}
func (l *Runtime) NavigationStatus(ctx context.Context) ServiceStatus {
	s := ServiceStatus{URL: l.JevURL, Models: []string{}}
	if l.JevToken == "" {
		s.Error = "The server's Jev connection is not configured."
		return s
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := l.call(ctx, l.JevURL, "/models", nil, &response, l.JevToken); err != nil {
		s.Error = err.Error()
		return s
	}
	s.Online = true
	for _, model := range response.Models {
		s.Models = append(s.Models, model.Name)
	}
	sort.Strings(s.Models)
	return s
}
func (l *Runtime) LanguageStatus(ctx context.Context) ServiceStatus {
	s := ServiceStatus{URL: l.OpenAIURL, Models: []string{}}
	if l.OpenAIKey == "" {
		s.Error = "The server's OpenAI connection is not configured."
		return s
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := l.call(ctx, l.OpenAIURL, "/models", nil, &response, l.OpenAIKey); err != nil {
		s.Error = err.Error()
		return s
	}
	s.Online = true
	for _, model := range response.Data {
		if strings.HasPrefix(model.ID, "gpt-") {
			s.Models = append(s.Models, model.ID)
		}
	}
	sort.Strings(s.Models)
	return s
}

// Each call is a fresh action proposal with server-rebuilt observations. No
// previous_response_id or client-supplied OpenAI conversation state is trusted.
func (l *Runtime) openAIAction(ctx context.Context, model, system string, state map[string]any, events []Event) (Action, error) {
	system += "\nFor this request select exactly one supplied function. Express the action using that function’s arguments, not a text message."
	a := Action{Input: state, Instructions: system}
	hasEvidence := false
	for _, event := range events {
		if event.Action.Type == "query" && event.Error == "" {
			hasEvidence = true
		}
	}
	tools := []any{}
	for _, spec := range [][3]string{{"search", "text", "Find entities by a label fragment."}, {"explore", "nodeId", "Inspect a known entity's connections."}, {"query", "query", "Execute a SPARQL query to obtain evidence."}, {"answer", "answer", "Write the final answer supported by successful query results."}} {
		if spec[0] == "answer" && !hasEvidence {
			continue
		}
		tools = append(tools, map[string]any{"type": "function", "name": spec[0], "description": spec[2], "strict": true, "parameters": map[string]any{"type": "object", "properties": map[string]any{"objective": map[string]string{"type": "string"}, spec[1]: map[string]string{"type": "string"}}, "required": []string{"objective", spec[1]}, "additionalProperties": false}})
	}
	raw, _ := json.Marshal(state)
	body := map[string]any{"model": model, "instructions": system, "input": string(raw), "tools": tools, "tool_choice": "required", "parallel_tool_calls": false, "max_output_tokens": 6000, "store": false}
	if strings.HasPrefix(model, "gpt-5") {
		body["reasoning"] = map[string]string{"effort": l.ReasoningEffort}
	}
	var response struct {
		Model  string `json:"model"`
		Status string `json:"status"`
		Output []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"output"`
	}
	if err := l.call(ctx, l.OpenAIURL, "/responses", body, &response, l.OpenAIKey); err != nil {
		return a, err
	}
	if response.Status != "completed" {
		return a, fmt.Errorf("OpenAI response was %s; no action was executed", response.Status)
	}
	calls := 0
	for _, item := range response.Output {
		if item.Type != "function_call" {
			continue
		}
		calls++
		if err := json.Unmarshal([]byte(item.Arguments), &a); err != nil {
			return a, errors.New("OpenAI returned invalid function arguments")
		}
		a.Type = item.Name
	}
	if calls != 1 {
		return a, errors.New("OpenAI must return exactly one action")
	}
	a.ActualModel = response.Model
	return validateAction(a)
}

func (l *Runtime) Trace(model string, a Action) *GenerationTrace {
	if a.ActualModel != "" {
		model = a.ActualModel
	}
	return &GenerationTrace{Model: model, Provider: "openai", Input: a.Input, Instructions: a.Instructions}
}
