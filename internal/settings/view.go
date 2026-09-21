// Package settings shows the config file as a page, opens it in the editor, and asks the daemon to read it again after a write
package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the config view
const ViewName = "config"

// hint is the second line of the page
const hint = "  e edit  R reload now  r refresh  q back"

// editorHint is the line about the editor files
const editorHint = "  K keymaps  o options  p plugins  t theme file  T pick a colorscheme"

// reloadTag is the text on an edit response that asks the plugin to reload after every write
const reloadTag = "reload"

// editorFile is one of the user's editor files under lua/user
type editorFile struct {
	// The file name under lua/user
	name string
	// What the plugin does after a write, source, theme, or restart
	onWrite string
}

// editorFiles maps the config page actions to the user files
var editorFiles = map[string]editorFile{
	"keymaps": {name: "keymaps.lua", onWrite: "source"},
	"options": {name: "options.lua", onWrite: "source"},
	"plugins": {name: "plugins.lua", onWrite: "restart"},
	"theme":   {name: "theme.lua", onWrite: "theme"},
}

// Settings is the config app
type Settings struct {
	// Returns the config file path
	path func() (string, error)
	// Asks the daemon to read the config again and rebuild its views, nil when reloading is not wired
	reload func() error
	// Returns the editor config directory, nil when the editor files are not wired
	editorDir func() (string, error)
	// Returns the shipped starter for a user file by name under lua/user, nil when not wired
	starter func(name string) ([]byte, error)
	// The config written when there is none, every key at its default
	template []byte
}

/**
 * New
 * Builds the view
 * @param path {func() (string, error)} - returns the config file path
 * @param reload {func() error} - reads the config again and rebuilds, nil for none
 * @return *Settings
 **/
func New(path func() (string, error), reload func() error) *Settings {
	return &Settings{path: path, reload: reload}
}

/**
 * WithTemplate
 * Sets the config text written when the file is missing
 * @param data {[]byte} - the full config with every key at its default
 * @return *Settings
 **/
func (s *Settings) WithTemplate(data []byte) *Settings {
	return &Settings{path: s.path, reload: s.reload, editorDir: s.editorDir, starter: s.starter, template: data}
}

/**
 * WithEditor
 * Wires the editor files, the directory they live in and the shipped starter written when one is missing
 * @param dir {func() (string, error)} - returns the editor config directory
 * @param starter {func(string) ([]byte, error)} - returns the starter for a file name under lua/user
 * @return *Settings
 **/
func (s *Settings) WithEditor(dir func() (string, error), starter func(string) ([]byte, error)) *Settings {
	return &Settings{path: s.path, reload: s.reload, editorDir: dir, starter: starter, template: s.template}
}

/**
 * Name
 * Returns config
 * @return string
 **/
func (s *Settings) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the home entry, which says whether the file parses
 * @return []view.Entry
 **/
func (s *Settings) Entries() []view.Entry {
	return []view.Entry{{Name: ViewName, Label: "config", Summary: func(context.Context) string {
		p, err := s.path()
		if err != nil {
			return err.Error()
		}
		if _, err := os.Stat(p); err != nil {
			return "none yet, open to write one"
		}
		if _, err := config.LoadFile(p); err != nil {
			return "does not parse"
		}
		return short(p)
	}}}
}

/**
 * Render
 * Shows the file's path, whether it parses, and what each section sets
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (s *Settings) Render(ctx context.Context) (view.Page, error) {
	p, err := s.path()
	if err != nil {
		return view.Page{}, err
	}
	lines := []string{"config  " + short(p), hint}
	if s.editorDir != nil {
		lines = append(lines, editorHint)
	}
	lines = append(lines, "")
	// A missing file
	if _, err := os.Stat(p); err != nil {
		lines = append(lines, "no config file yet, e writes one with every key at its default")
		return page(lines), nil
	}
	// A file that does not parse shows the error and nothing else
	cfg, err := config.LoadFile(p)
	if err != nil {
		lines = append(lines, "status: does not parse, the daemon keeps the last good config", "", "  "+strings.TrimPrefix(err.Error(), p+": "))
		return page(lines), nil
	}
	lines = append(lines, "status: ok", "")
	lines = append(lines, summary(cfg)...)
	return page(lines), nil
}

/**
 * RenderPath
 * Has no sub pages
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below config
 * @return view.Page, error
 **/
func (s *Settings) RenderPath(ctx context.Context, path string) (view.Page, error) {
	return view.Page{}, fmt.Errorf("config: no page %q", path)
}

/**
 * Act
 * Runs an action from the config page, edit opens the file and writes the template first when there is none, reload asks the daemon to read it again
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (s *Settings) Act(ctx context.Context, a view.Action) (view.Response, error) {
	switch a.Name {
	case "refresh", "open":
		p, err := s.Render(ctx)
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return view.Show(p), nil
	case "edit":
		p, err := s.ensure()
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return view.Response{Kind: view.KindEdit, Path: p, Text: reloadTag}, nil
	case "keymaps", "options", "plugins", "theme":
		p, err := s.editorFile(editorFiles[a.Name])
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return view.Response{Kind: view.KindEdit, Path: p, Text: editorFiles[a.Name].onWrite}, nil
	case "reload":
		if s.reload == nil {
			return view.Fail("config: reloading is not available here"), nil
		}
		if err := s.reload(); err != nil {
			return view.Fail(err.Error()), nil
		}
		p, err := s.Render(ctx)
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		r := view.Show(p)
		r.Text = "config reloaded"
		return r, nil
	}
	return view.Fail("config: unknown action " + a.Name), nil
}

/**
 * ensure
 * Returns the config path, writing the template with owner only permissions when the file is missing
 * @return string, error
 **/
func (s *Settings) ensure() (string, error) {
	p, err := s.path()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return p, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(p, s.template, 0o600); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	return p, nil
}

/**
 * editorFile
 * Returns the path of a user editor file, writing the shipped starter when it is missing
 * @param f {editorFile} - the file
 * @return string, error
 **/
func (s *Settings) editorFile(f editorFile) (string, error) {
	if s.editorDir == nil || s.starter == nil {
		return "", errors.New("config: the editor files are not available here")
	}
	dir, err := s.editorDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "lua", "user", f.name)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("config: %w", err)
	}
	data, err := s.starter(f.name)
	if err != nil {
		return "", fmt.Errorf("config: no starter for %s: %w", f.name, err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}
	return p, nil
}

/**
 * summary
 * Lists what each section sets, one line per section
 * @param c {config.Config} - the config
 * @return []string
 **/
func summary(c config.Config) []string {
	names := make([]string, 0, len(c.Mail.Accounts))
	for _, a := range c.Mail.Accounts {
		names = append(names, a.Name)
	}
	mail := "none"
	if len(names) > 0 {
		mail = strings.Join(names, ", ")
	} else if c.Mail.Maildir != "" {
		mail = short(c.Mail.Maildir)
	}
	roots := make([]string, 0, len(c.Projects.RootDirs()))
	for _, r := range c.Projects.RootDirs() {
		roots = append(roots, short(r))
	}
	discord := "no token file"
	if c.Discord.TokenFile != "" {
		discord = short(c.Discord.TokenFile)
	}
	chrome := "found on the path"
	if c.Browser.Chrome != "" {
		chrome = c.Browser.Chrome
	}
	search := "local searxng"
	if c.Browser.Search != "" {
		search = c.Browser.Search
	}
	contracts := "default"
	if c.Geas.Contracts != "" {
		contracts = short(c.Geas.Contracts)
	}
	days := "default"
	if c.Calendar.Days > 0 {
		days = fmt.Sprintf("%d", c.Calendar.Days)
	}
	editor := "symphony's own"
	if c.Editor.Own() {
		editor = "your usual nvim config"
	}
	return []string{
		"editor: " + editor,
		"mail accounts: " + mail,
		"calendar days: " + days,
		"projects roots: " + strings.Join(roots, ", "),
		"discord: " + discord,
		"browser: " + chrome + ", search " + search,
		"geas contracts: " + contracts,
	}
}

/**
 * page
 * Wraps lines into the config page
 * @param lines {[]string} - the lines
 * @return view.Page
 **/
func page(lines []string) view.Page {
	return view.Page{Name: ViewName, Title: "config", Lines: lines, Keys: make([]string, len(lines)), Filetype: "config"}
}

/**
 * short
 * Replaces the home directory with a tilde
 * @param p {string} - the path
 * @return string
 **/
func short(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home) {
		return p
	}
	return "~" + strings.TrimPrefix(p, home)
}
