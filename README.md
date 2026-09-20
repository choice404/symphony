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

```
make build
./target/symphony
```

make build runs these two, one binary each, if you would rather type them yourself.

```
go build -o target/symphony ./cmd/symphony
go build -o target/symphonyd ./cmd/symphonyd
```

Anything after the binary name is passed to nvim, so ./target/symphony --clean starts without your config. Quit with :q like normal.

Symphony opens on a home buffer that lists every app with a summary, unless you named a file on the command line in which case it opens that file the way nvim would. In any symphony buffer <CR> opens the item under the cursor, r renders the page again, q goes back to the previous buffer, and gh jumps home. :Symphony mail, :Symphony home, and :Symphony open <view> get you anywhere by name.

# The daemon

symphonyd holds the views and answers over a unix socket at $XDG_RUNTIME_DIR/symphony/symphonyd.sock, or under ~/.cache/symphony when there is no runtime dir, and SYMPHONY_SOCKET overrides both. The TUI dials it on start and spawns one in its own session when nothing answers, so the daemon keeps running after you quit and the next start attaches to the same one. symphonyd status pings it and symphonyd stop asks it to exit. The socket and its directory are mode 600 and 700 so only you can connect.

Because the socket speaks msgpack rpc, a normal nvim can use it too. Put nvim/symphony.nvim on your runtimepath like any other plugin, start symphonyd, and :Symphony mail dials the socket and shows the inbox in that nvim, no TUI needed. :Symphony connect takes a socket path when it is not the default, :Symphony status says whether you are attached, and :checkhealth symphony shows the socket and pings the daemon.

# Contracts

Service plugins are geas contracts (github.com/choice404/geas). The mail service is one, contracts/mail.geas: the maildir is a vow, list and open are pledges the daemon binds to Go, configured is a pledge whose body runs in geas, and the requirements block turns the runtime's state into the health you see on the home page, signed, partial, fulfilled, or broken with the error that broke it. The plain build never touches the runtime. make geas builds the contracts with the geas compiler, links libgeasrt in through cgo with the geas build tag, and copies the modules beside the binaries where the daemon looks for them, or set contracts under [geas] in the config to point somewhere else.

```
make geas
./target/symphony
``` When no module is found the daemon logs it and mail runs the plain way.

# Config

The config lives at ~/.config/symphony/config.toml and is optional, a missing file is the empty config.

```
[mail]
maildir = "~/Mail/inbox"

[geas]
contracts = "~/.local/share/symphony/contracts"
```

Mail reads a Maildir straight off disk, the cur and new folders, so anything that syncs one works, such as mbsync, offlineimap, etc. The inbox page lists messages newest first with N for unread, F for flagged, and R for replied, and opening one shows the headers and the plain text part, or the html part with its tags stripped when there is no plain one. Without a maildir the mail page says how to set one.

# Test

```
make test
```

That is the Go suite under the race detector and the plugin tests, which are these two.

```
go test -race ./...
nvim --headless -u NONE -l nvim/symphony.nvim/tests/run.lua
```

make test-geas runs the suite with the geas packages included and make lint runs gofmt, vet, and golangci-lint for both builds.

The Go tests spawn a real nvim with --clean and check the screen, the cursor, a resize, the plugin round trip, and that quitting is reported. They skip when nvim is not on the path.

# Status

Step 4 of 5, nvim embedded and drawn, every app is a page the plugin shows as a buffer, the daemon serves them over a socket, and mail runs as a geas contract when built with make geas. Next is a dusk plugin body behind abi c.
