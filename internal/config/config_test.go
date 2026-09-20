package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMissingIsEmpty(t *testing.T) {
	c, err := LoadFile(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || c.Mail.Maildir != "" {
		t.Fatalf("config = %+v err = %v", c, err)
	}
}

func TestLoadFileExpandsHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mail]\nmaildir = \"~/Mail/inbox\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mail.Maildir != "/home/tester/Mail/inbox" {
		t.Fatalf("maildir = %q", c.Mail.Maildir)
	}
}

func TestLoadFileBadToml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[mail\nmaildir = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected parse error")
	}
}
