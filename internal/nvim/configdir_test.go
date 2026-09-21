package nvim

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestInstallConfigKeepsUserFilesAndLock(t *testing.T) {
	// Point the config directory at a scratch one
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tree := fstest.MapFS{
		"cfg/init.lua":             {Data: []byte("one")},
		"cfg/lua/plugins/a.lua":    {Data: []byte("a")},
		"cfg/" + lockFile:          {Data: []byte("{}")},
		"cfg/lua/user/keymaps.lua": {Data: []byte("starter")},
	}
	dest, err := InstallConfig(tree, "cfg")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "init.lua")); string(got) != "one" {
		t.Fatalf("init = %q", got)
	}
	// A user file and an edited lock survive, a changed shipped file is written again
	if err := os.WriteFile(filepath.Join(dest, "lua", "plugins", "mine.lua"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, lockFile), []byte("{\"x\":1}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "lua", "user", "keymaps.lua"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree["cfg/init.lua"] = &fstest.MapFile{Data: []byte("two")}
	tree["cfg/lua/user/keymaps.lua"] = &fstest.MapFile{Data: []byte("newer starter")}
	if _, err := InstallConfig(tree, "cfg"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "init.lua")); string(got) != "two" {
		t.Fatalf("init after = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "lua", "plugins", "mine.lua")); string(got) != "mine" {
		t.Fatalf("user file = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, lockFile)); string(got) != "{\"x\":1}" {
		t.Fatalf("lock = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "lua", "user", "keymaps.lua")); string(got) != "edited" {
		t.Fatalf("user starter = %q", got)
	}
}
