package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigratesLegacyConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SYNCSCOPE_CONFIG_DIR", "")
	t.Setenv("ARGODECK_CONFIG_DIR", "")
	t.Setenv("SYNCSCOPE_NO_KEYRING", "1")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	old := filepath.Join(base, legacyName)
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "config.json"), []byte(`{"contexts":[{"id":"x","name":"prod","server":"https://a"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir() != filepath.Join(base, "syncscope") || len(s.Contexts()) != 1 || s.Contexts()[0].Name != "prod" {
		t.Fatalf("not migrated: dir=%s contexts=%+v", s.Dir(), s.Contexts())
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("legacy dir should have been moved")
	}
}
