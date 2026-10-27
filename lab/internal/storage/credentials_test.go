package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
)

func TestCredentialsPersistPrivately(t *testing.T) {
	store := FileCredentials{Path: filepath.Join(t.TempDir(), "model-credentials.json")}
	empty, err := store.Load()
	if err != nil || empty.OpenAIKey != "" {
		t.Fatal("missing configuration must allow onboarding", err)
	}
	want := models.Credentials{OpenAIKey: "test-openai", JevToken: "test-jev"}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got != want {
		t.Fatal("credentials did not survive reload", err)
	}
	info, err := os.Stat(store.Path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credentials are not owner-only", err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.Path))
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary credential file left behind", err)
	}
}
func TestCredentialsRejectSymlinkOnLoad(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "model-credentials.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := (FileCredentials{Path: link}).Load(); err == nil {
		t.Fatal("followed credential symlink")
	}
}
