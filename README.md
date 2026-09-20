# symphony

An all in one TUI for using a computer the way I want, vim style.

[img](path)

# TOC

- [ ]()

# About

Symphony started because I wanted one place to do everything from a terminal, email, browser, coding, project management, git, calendar, google workspace, docs, spreadsheets, discord, etc. without giving up the way vim feels. I know emacs exists but I come from a heavy vim background so instead of learning a whole other editor I'm building the thing around neovim itself. Symphony embeds nvim and uses it as the modal engine for every view, so a mail inbox or a calendar is a buffer with my own keymaps and my own plugins, and bubbletea draws the chrome around the nvim grid. It may not be perfect but it's better than nothing.

# How it works

The Go program in cmd/symphony installs the lua plugin from nvim/symphony.nvim into ~/.cache/symphony, starts nvim --embed with that directory on the runtimepath, attaches as a linegrid ui, and applies every redraw event to a grid in internal/nvim. On each flush a copy of the grid is handed to the bubbletea model in internal/ui which renders it with true color escapes, and every key the terminal sends is translated to nvim key notation and forwarded in order. The plugin finds the host through vim.g.symphony_channel and calls back with rpcrequest, which is how :Symphony ping gets its pong. Later a daemon, symphonyd, will own the network I/O, the sqlite cache, and the service plugins written as geas contracts with bodies in geas, C, or dusk, and the TUI will talk to it over a unix socket so it can run headless on a server.

# Build and run

go build -o target/symphony ./cmd/symphony
./target/symphony

Anything after the binary name is passed to nvim, so ./target/symphony --clean starts without your config. Quit with :q like normal.

# Test

go test -race ./...
nvim --headless -u NONE -l nvim/symphony.nvim/tests/run.lua

The Go tests spawn a real nvim with --clean and check the screen, the cursor, a resize, the plugin round trip, and that quitting is reported. They skip when nvim is not on the path.

# Status

Step 1 of 5, nvim embedded and drawn, plugin round trip working. Next is mail in plain go pushed into a buffer, then the daemon split, then the geas binding, then a dusk plugin body.
