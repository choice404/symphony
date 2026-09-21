package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

/**
 * RunHeaded
 * Opens a real browser window on the profile at a url and waits until it is closed, for a login a text page cannot do such as a captcha or a passkey
 * @param ctx {context.Context} - the context
 * @param chrome {string} - the browser binary
 * @param profile {string} - the user data directory, the same one the engine uses
 * @param url {string} - where to start
 * @return error
 **/
func RunHeaded(ctx context.Context, chrome, profile, url string) error {
	if chrome == "" {
		return fmt.Errorf("browser: no chromium found")
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	// The window, the profile, and nothing else
	cmd := exec.CommandContext(ctx, chrome, "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--new-window", url)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("browser: %w", err)
	}
	return nil
}
