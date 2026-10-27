package models

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
)

type memoryCredentials struct {
	value Credentials
	fail  bool
}

func (s *memoryCredentials) Load() (Credentials, error) { return s.value, nil }
func (s *memoryCredentials) Save(value Credentials) error {
	if s.fail {
		return errors.New("storage unavailable")
	}
	s.value = value
	return nil
}

func TestFailedCredentialSaveRetainsConfiguration(t *testing.T) {
	runtime := New()
	store := &memoryCredentials{value: Credentials{OpenAIKey: "old-openai", JevToken: "old-jev"}}
	if err := runtime.AttachCredentials(store); err != nil {
		t.Fatal(err)
	}
	store.fail = true
	if err := runtime.SaveCredentials(Credentials{OpenAIKey: "new-openai"}); err == nil {
		t.Fatal("failed storage accepted")
	}
	if runtime.snapshot().OpenAIKey != "old-openai" {
		t.Fatal("failed save changed active key")
	}
}
func TestMissingCredentialsFailWithoutAPIRequests(t *testing.T) {
	runtime := New()
	if _, err := runtime.Next(context.Background(), domain.Seeds()[0], "Who?", "gpt-5", "", "en", nil); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := runtime.Rank(context.Background(), domain.Seeds()[0], "Who?", "jev-latest"); err == nil {
		t.Fatal("missing navigation key accepted")
	}
}
func TestCredentialUpdatesAndSnapshotsAreConcurrentSafe(t *testing.T) {
	runtime := New()
	if err := runtime.AttachCredentials(&memoryCredentials{}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for range 50 {
			if err := runtime.SaveCredentials(Credentials{OpenAIKey: "new-openai", JevToken: "new-jev"}); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer group.Done()
		for range 50 {
			runtime.PublicConfig()
			runtime.Models()
			runtime.snapshot()
		}
	}()
	group.Wait()
}
