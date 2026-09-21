package symphony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/choice404/symphony/internal/config"
)

func TestExampleConfigParsesToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, ExampleConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every blank or zero falls through to the default
	if c.Mail.ShowLimit() != config.DefaultLimit || c.Calendar.Ahead() != config.DefaultDays || len(c.Mail.Accounts) != 0 {
		t.Fatalf("defaults = %+v", c)
	}
	if c.Editor.Own() || c.Browser.Chrome != "" || c.Discord.TokenFile != "" || c.Geas.Contracts != "" {
		t.Fatalf("blanks = %+v", c)
	}
	if roots := c.Projects.RootDirs(); len(roots) != 1 || filepath.Base(roots[0]) != "projects" {
		t.Fatalf("roots = %v", roots)
	}
}
