package models

import (
	"errors"
	"strings"
)

// Credentials are private server configuration. Public responses expose only booleans.
type Credentials struct {
	OpenAIKey string `json:"openaiApiKey"`
	JevToken  string `json:"jevApiKey"`
}
type CredentialStore interface {
	Load() (Credentials, error)
	Save(Credentials) error
}

func (l *Runtime) snapshot() *Runtime {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return &Runtime{JevURL: l.JevURL, OpenAIURL: l.OpenAIURL, DefaultModel: l.DefaultModel, DefaultNavigationModel: l.DefaultNavigationModel, ReasoningEffort: l.ReasoningEffort, JevToken: l.JevToken, OpenAIKey: l.OpenAIKey, Client: l.Client}
}

func (l *Runtime) AttachCredentials(store CredentialStore) error {
	saved, err := store.Load()
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if saved.OpenAIKey != "" {
		l.OpenAIKey = saved.OpenAIKey
	}
	if saved.JevToken != "" {
		l.JevToken = saved.JevToken
	}
	l.credentials = store
	return nil
}

// Blank fields retain the current secret. Persist before making changes visible.
func (l *Runtime) SaveCredentials(update Credentials) error {
	update.OpenAIKey = strings.TrimSpace(update.OpenAIKey)
	update.JevToken = strings.TrimSpace(update.JevToken)
	for _, key := range []string{update.OpenAIKey, update.JevToken} {
		if len(key) > 4096 || strings.ContainsAny(key, "\r\n\x00") {
			return errors.New("invalid API key format")
		}
	}
	if update.OpenAIKey == "" && update.JevToken == "" {
		return errors.New("enter at least one API key")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	next := Credentials{OpenAIKey: l.OpenAIKey, JevToken: l.JevToken}
	if update.OpenAIKey != "" {
		next.OpenAIKey = update.OpenAIKey
	}
	if update.JevToken != "" {
		next.JevToken = update.JevToken
	}
	if l.credentials == nil {
		return errors.New("credential storage is not configured")
	}
	if err := l.credentials.Save(next); err != nil {
		return errors.New("could not save API keys; existing configuration was retained")
	}
	l.OpenAIKey, l.JevToken = next.OpenAIKey, next.JevToken
	return nil
}
