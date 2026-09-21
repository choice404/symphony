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

func TestAllAccounts(t *testing.T) {
	// A lone maildir is one unnamed account
	m := Mail{Maildir: "/m"}
	if got := m.All(); len(got) != 1 || got[0].Name != "" || got[0].Maildir != "/m" {
		t.Fatalf("single = %+v", got)
	}
	// A list wins over the maildir
	m = Mail{Maildir: "/m", Accounts: []Account{{Name: "personal", Maildir: "/p"}, {Name: "school", Maildir: "/s"}}}
	if got := m.All(); len(got) != 2 || got[0].Name != "personal" || got[1].Maildir != "/s" {
		t.Fatalf("list = %+v", got)
	}
	// Nothing is nothing
	if got := (Mail{}).All(); got != nil {
		t.Fatalf("empty = %+v", got)
	}
}

func TestLoadFileAccounts(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[[mail.accounts]]\nname = \"personal\"\nmaildir = \"~/Mail/personal\"\n\n[[mail.accounts]]\nname = \"school\"\nmaildir = \"~/Mail/school\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Mail.All()
	if len(got) != 2 || got[1].Name != "school" || got[1].Maildir != "/home/tester/Mail/school" {
		t.Fatalf("accounts = %+v", got)
	}
}

func TestShowLimit(t *testing.T) {
	if (Mail{}).ShowLimit() != DefaultLimit || (Mail{Limit: -1}).ShowLimit() != DefaultLimit {
		t.Fatal("unset limit should be the default")
	}
	if (Mail{Limit: 50}).ShowLimit() != 50 {
		t.Fatal("set limit should hold")
	}
}

func TestAccountDefaults(t *testing.T) {
	a := Account{}
	if a.Synced() || a.IMAPHost() != "imap.gmail.com:993" || a.FetchCount() != DefaultFetch || a.SyncEvery() != 0 {
		t.Fatalf("defaults = %v %q %d %v", a.Synced(), a.IMAPHost(), a.FetchCount(), a.SyncEvery())
	}
	a = Account{Auth: "oauth", Host: "mail.example.com", Fetch: 10, Sync: "15m"}
	if !a.Synced() || a.IMAPHost() != "mail.example.com:993" || a.FetchCount() != 10 || a.SyncEvery().Minutes() != 15 {
		t.Fatalf("set = %v %q %d %v", a.Synced(), a.IMAPHost(), a.FetchCount(), a.SyncEvery())
	}
	if (Account{Host: "h:1143"}).IMAPHost() != "h:1143" {
		t.Fatal("a port in the host should be kept")
	}
	if (Account{Sync: "junk"}).SyncEvery() != 0 {
		t.Fatal("an unreadable interval should be zero")
	}
}
