package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvIsDataAndExistingEnvironmentWins(t *testing.T) {
	t.Setenv("SPINACH_TEST_EXISTING", "process")
	for _, key := range []string{"SPINACH_TEST_LITERAL", "SPINACH_TEST_QUOTED"} {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("export SPINACH_TEST_EXISTING=file\nSPINACH_TEST_LITERAL='$(touch /no-command-execution)'\nSPINACH_TEST_QUOTED=\"a # value\" # comment\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SPINACH_TEST_EXISTING") != "process" || os.Getenv("SPINACH_TEST_LITERAL") != "$(touch /no-command-execution)" || os.Getenv("SPINACH_TEST_QUOTED") != "a # value" {
		t.Fatal("incorrect environment parsing")
	}
}
func TestEnvErrorsNeverIncludeSecretValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("BROKEN=\"private-token\n"), 0600)
	err := LoadEnv(path)
	if err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatal("unsafe or missing parse error")
	}
}
