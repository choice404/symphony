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

symphonyd holds the views and answers over a unix socket at $XDG_RUNTIME_DIR/symphony/symphonyd.sock, or under ~/.cache/symphony when there is no runtime dir, and SYMPHONY_SOCKET overrides both. The TUI dials it on start and spawns one in its own session when nothing answers, so the daemon keeps running after you quit and the next start attaches to the same one. When you rebuild, the TUI notices the running daemon came from an older binary, asks it to stop, and starts the new one, so you never talk to stale code. The config file is read again on every render, so editing it lands on the next refresh without touching the daemon. symphonyd status pings it and symphonyd stop asks it to exit. The socket and its directory are mode 600 and 700 so only you can connect.

make install copies the last built binaries into ~/.local/bin and the contract modules into ~/.local/share/symphony/contracts, which is where the daemon looks when the config does not say otherwise.

Because the socket speaks msgpack rpc, a normal nvim can use it too. Put nvim/symphony.nvim on your runtimepath like any other plugin, start symphonyd, and :Symphony mail dials the socket and shows the inbox in that nvim, no TUI needed. :Symphony connect takes a socket path when it is not the default, :Symphony status says whether you are attached, and :checkhealth symphony shows the socket and pings the daemon.

# Projects

Home starts with a projects entry. The page lists every directory one level under your project roots, ~/projects unless [projects] roots says otherwise, with the branch and how many paths are dirty, the last commit and how long ago, and when you last opened it through symphony, most recently opened first. <CR> enters the project in a tab of its own with the tab's directory set to it, so the first tab keeps its own directory, and the tab shows the project's file tree, an ls -al style listing with the mode, size, and time of each entry, directories first. <CR> on a directory expands it in place and on a file opens it in that tab, l expands, h collapses the directory or the one the cursor is inside, a dot shows dotfiles, f brings up whatever file picker you have, Telescope, fzf-lua, oil, neo-tree, or netrw, and r reads the tree again. From a file it is your own nvim, your config, your plugins, your keymaps.

:q climbs back one level. On a file it drops the file and returns to the tree, on the tree it closes the tab and lands on the projects page. ZZ, ZQ, :wq, and :x do the same, the first three writing first where there is something to write. Unsaved changes stop a plain :q with the same E37 vim gives, :q! goes anyway, and :qa still quits symphony itself. A split closes on :q the way it always did, and :q in another tab is vim's own. :Symphony leave closes the project from anywhere.

c opens a new project page with the name, the root, and whether to git init, and gs makes the directory with a README and enters it. D forgets a project's opened time, it does not touch the directory.

# Git

g on a project's tree opens its status page, the branch with how far ahead or behind the upstream it is, then the staged, unstaged, and untracked paths. <CR> on a path shows its diff, the staged part and the unstaged part, with diff colors. s stages the path and u takes it back out. c opens a commit page when something is staged, the message goes above the marker and the staged stat sits below it, and gs commits. l is the log, the newest two hundred commits, <CR> on one shows its patch and gs comes back to the status. p pushes and P pulls with a fast forward only, and whatever git printed lands on a page so a rejected push or a conflict is read in full. Everything runs through git itself in the daemon, and a path from a page is checked to sit inside the repository before git sees it.

```
[projects]
roots = ["~/projects", "~/work"]
```

# Discord

Discord runs through a bot, since automating a normal account is against Discord's terms and gets the account banned. Make an application at discord.com/developers, add a bot to it, turn on the Message Content intent under Bot, and invite the bot to your servers with permission to read and send messages. Put the token in ~/.config/symphony/discord.token mode 600, or name another file with token_file under [discord], and restart the daemon. Home then shows the server count and how many messages arrived since you last looked.

The discord page lists the servers, <CR> opens one to its text channels grouped by category with the unread counts, and <CR> on a channel shows its last fifty messages oldest first, grouped by day, your bot's own lines marked you. The daemon keeps the gateway open so new messages flow in as they happen, r shows them, and i asks for a line and sends it as the bot. What the bot cannot see, your own direct messages and servers it is not in, is not there, that is the trade for staying inside the rules.

```
[discord]
token_file = "~/.config/symphony/discord.token"
```

# The editor

Symphony runs its own nvim config, a LazyVim distro the binary writes to ~/.config/symphony/nvim and runs under NVIM_APPNAME symphony/nvim, so its plugins, data, and cache sit beside symphony's own and never touch your usual nvim. The first start clones lazy.nvim and the plugins pinned in lazy-lock.json, which needs the network once, after that it is offline. The landing screen is a dashboard with every app and its live summary from the daemon, one key each, p projects, m mail, c calendar, d discord, u discord with your account, b browser, e config, l plugins, q quit, and q on the last page of an app returns to it. The same apps sit under leader y with which-key naming them. vim-tmux-navigator is in, so ctrl h j k l cross splits and tmux panes alike.

Your own keymaps, options, plugins, and colors live in ~/.config/symphony/nvim/lua/user, four starter files written once and never touched by an update, loaded last so they win. The config page opens them, k keymaps, o options, p plugins, t the theme file, and every write applies the file on the spot, keymaps and options are sourced again, the theme is applied, new plugins wait for the next start. T on the config page, or :Symphony theme, picks a colorscheme from the installed ones with a preview and writes the choice into theme.lua, which also takes the background and any highlight groups to set on top. A colorscheme plugin goes in plugins.lua first. Anything dropped into lua/plugins is picked up too and never overwritten, and the shipped files are written again when a new binary changes them. lazy-lock.json is written once, :Lazy update moves it and :Lazy restore puts it back to the shipped pins. To run your usual config inside symphony instead:

```
[editor]
config = "own"
```

# Config from inside

The first run writes ~/.config/symphony/config.toml with every key present at its default or blank, mode 600, the same file as config.example.toml in the repo, so the reference for every setting is the file itself. A blank string or a zero means unset and the default applies. config on home shows the file's path, whether it parses, and what each section sets. e opens the file in the editor. Every write of that buffer makes the daemon read the file again and build its views over it, so a new mail account, project root, or search engine is live on the next page, and a file that does not parse comes back as an error in the message line while the daemon keeps the last good config. R reloads without a write, and :Symphony config opens the file straight away. :q, :wq, ZZ, and ZQ on a file a page opened go back to that page, the same way a project file goes back to its tree, :qa still quits.

# Browser

The daemon runs a headless Chromium, google-chrome, chromium, or brave, whichever is on the path or the one chrome names under [browser], with a profile of its own under ~/.local/share/symphony/browser so logins survive. The browser page lists the open tabs, o asks for a url and / for a search, and a tab page is the site read as text, headings marked, every link numbered like [3], every field shown like {2 email: _} and every button like {4 Log in}. <CR> on a link line follows it, <CR> on a field line asks for a value and types it, on a button line clicks it, f asks for a link number, gs presses Enter in the field under the cursor, b and F go back and forward, r reloads, x closes the tab. :Symphony browser example.com opens a url straight away.

A search goes to the url named by search under [browser] with the query appended, and when nothing is named it goes to a SearXNG on this machine at http://127.0.0.1:8888/search?q= since the public engines answer a headless browser with a captcha. The quickest SearXNG is the container:

```
docker run -d --name searxng --restart unless-stopped -p 8888:8080 -v ~/.config/searxng:/etc/searxng docker.io/searxng/searxng
```

Any instance works, and both settings are optional:

```
[browser]
chrome = "/usr/bin/chromium"
search = "https://searx.example.org/search?q="
```

Some logins need a real window once, a captcha, a passkey, a security key. symphonyd browser login <url> asks the daemon to let go of the profile, opens a visible browser window on it, and waits until you close the window. The daemon starts its headless browser again on the next page, now logged in.

# Discord, your own account

discord (you) on home is Discord's own web client running inside that browser, so it is your account and the official client makes every request, the same footing as any desktop wrapper. The first open lands on the login page, so run this once and log in with your normal credentials, the captcha and the passkey work in the window:

```
symphonyd browser login https://discord.com/login
```

After that the page lists your direct messages and your servers in the sidebar's order, a folder shows as + when closed and - when open with the servers inside it indented, <CR> on a folder opens or closes it in the client, <CR> opens a server to its channels with unread marked, <CR> on a channel or a direct message shows the visible chat grouped by day, i asks for a line and puts it into Discord's own message box as one insert, closes any picker the text raised, and presses Enter, and the page reads again once the box has emptied, so a message that stays in the box comes back as an error instead of a silent miss. Everything comes from the client's page as it stands, so r reads it again and there is no history beyond what the client has rendered. The bot page stays for anything the daemon does on its own, such as unread counts on home while nothing is open.

The client's page structure is Discord's and changes without notice, so a page that comes up empty after an update means the readers need a refresh, not that the login is gone.

# Calendar

Every account with auth = oauth gets a calendar, the same login covers mail and calendar so one symphonyd authorize does both, and an account authorized before the calendar scope was added needs one more authorize. Home lists an agenda per account and an all agenda, each showing today's count and the next event. An agenda is the next thirty days grouped by day, days under [calendar] changes the span, ]d and [d move a week, gt comes back to today, ]a [a and ga move between accounts, <CR> opens the event with its attendees and link, and D deletes it on the server. c opens a new event page, an editable buffer with the title, start, end, all day, and location fields above a marker and the description below, and gs saves it to the account's own calendar. Times are typed as 2026-03-01 14:00, or as 2026-03-01 with all day set to yes.

The daemon keeps a cache per account under ~/.cache/symphony/calendar and warms it on the account's sync interval, so agendas open instantly and read fine offline, r fetches now, and symphonyd calendar sync warms every account from the command line.

```
[calendar]
days = 30
```

# Contracts

Service plugins are geas contracts (github.com/choice404/geas). The mail service is one, contracts/mail.geas: the maildir is a vow, list and open are pledges the daemon binds to Go, configured is a pledge whose body runs in geas, and the requirements block turns the runtime's state into the health you see on the home page, signed, partial, fulfilled, or broken with the error that broke it. The plain build never touches the runtime. make geas builds the contracts with the geas compiler, links libgeasrt in through cgo with the geas build tag, and copies the modules beside the binaries where the daemon looks for them, or set contracts under [geas] in the config to point somewhere else.

```
make geas
./target/symphony
```

When no module is found the daemon logs it and mail runs the plain way.

# Plugin bodies in dusk

A pledge body does not have to be geas or Go. plugins/mail/classify.dusk is the classify pledge of the mail contract written in dusk (github.com/choice404/dusk), it labels every inbox line personal, notice, list, money, or urgent from the sender and subject. dusk build --lib turns it into a static archive, the daemon links that archive in with the dusk build tag and binds the exported function straight to the pledge, no trampoline, and glue.c beside it is the only code that touches the geas value layout, so the dusk side reads two strings and returns one. dusk does not emit a position independent shared object yet, which is why the body is linked in at build time instead of loaded from a file.

```
make dusk
./target/symphony
```

Without the dusk tag the daemon logs that classify is not linked in and inbox lines carry no label.

# Config

The config lives at ~/.config/symphony/config.toml and is optional, a missing file is the empty config. One account is a maildir under [mail], more than one is a [[mail.accounts]] table each, and when both are present the list wins. The daemon reads the file again on every render, so an edit lands on the next r.

```
[[mail.accounts]]
name = "personal"
maildir = "~/Mail/personal"

[[mail.accounts]]
name = "school"
maildir = "~/Mail/school"

[[mail.accounts]]
name = "proton"
maildir = "~/Mail/proton"

[mail]
limit = 500

[geas]
contracts = "~/.local/share/symphony/contracts"
```

limit is how many of the newest messages the inbox shows across all accounts, 500 when unset, since a synced Gmail inbox can hold tens of thousands and nobody scrolls that far. Home lists each account and, with more than one, an all accounts entry. Every account has its own inbox, sent, and spam pages, and the all pages merge every account newest first with an account column. A list header carries a tag per account, its contract state or its error, so one broken account never hides the others. Each account is its own signed Mailbox contract, so the health you see is per account.

# Reading, writing, and cleaning up mail

In a list <CR> opens the message, ]a and [a step through the accounts and back to all, ga jumps to the all accounts page, gi, gs, and gS switch to the inbox, the sent folder, and the spam review page for the same account, r renders the page again, and q goes back the way you came. :Symphony mail school sent opens a page by name.

c opens a compose page for the account you are on, R replies to the message under the cursor or the one you are reading, with the address, the subject, the quoted body, and the thread headers filled in, and F forwards it. A compose page is an ordinary editable buffer, the headers sit above a marker line and the body below it, so you edit it like any file, then gs sends it through the daemon over SMTP and the buffer closes. Gmail files what you send into the sent folder itself and the daemon syncs it back a few seconds later. Sending needs the account to have auth set, the same login the sync uses.

D moves the message under the cursor to the trash on the server and removes the local copy, which on Gmail is the Trash folder with its thirty day grace, so nothing is gone for good from symphony.

The spam page asks Jev, TypeSafe's judgment model, whether each message in the inbox is spam, one probability per message, cached so a message is judged once. It lists the ones at or above one half, most likely first, with a mark column. d marks or unmarks the line, x trashes every marked message after you have looked them over, and nothing is ever trashed without a mark you put there. The key goes in TYPESAFE_API_KEY or in ~/.config/symphony/typesafe.key mode 600, and without one the page says so and everything else keeps working.



# Mail accounts and MFA

Mail reads a Maildir straight off disk, the cur and new folders, so anything that syncs one works, such as mbsync, offlineimap, etc. The inbox page lists messages newest first with N for unread, F for flagged, and R for replied, and opening one shows the headers and the plain text part, or the html part with its tags stripped when there is no plain one.

MFA never touches IMAP. A YubiKey or an Okta push protects the browser login to the account, and mail sync uses something you get after that login, either an app password or an OAuth2 token, and which one depends on the provider. The three kinds of account I have are below, personal Gmail with a YubiKey, university Google Workspace behind Okta, and Proton Mail with a YubiKey.

Sync runs through mbsync for now. Install it with paru -S isync, and the config is ~/.mbsyncrc. Secrets go in files under ~/.config/symphony with mode 600, never in the config itself and never on a command line.

## Personal Gmail, app password

An app password is a 16 character password Google issues for one client, and it needs 2-step verification on, which a YubiKey account already has. The key stays the login factor and IMAP never sees it. Make one under Google account, Security, 2-Step Verification, App passwords, and put it in a file.

```
IMAPAccount personal
Host imap.gmail.com
User you@gmail.com
PassCmd "cat ~/.config/symphony/personal.pass"
TLSType IMAPS

IMAPStore personal-remote
Account personal

MaildirStore personal-local
Path ~/Mail/personal/
Inbox ~/Mail/personal/inbox
SubFolders Verbatim

Channel personal
Far :personal-remote:"INBOX"
Near :personal-local:inbox
Create Near
Expunge Both
SyncState *
MaxMessages 2000
MaxSize 10m
ExpireUnread yes
```

MaxMessages keeps only the newest 2000 locally, which matters on Gmail where an inbox holds tens of thousands and IMAP is capped at about 2500 MB a day, so a full first sync gets cut off with an OVERQUOTA error. MaxSize skips anything over 10 MB. Then mbsync personal, and the maildir in the symphony config is ~/Mail/personal/inbox. Gmail also takes OAuth2 the same way the school account does below, which is what symphony will use itself once it syncs on its own.

## University Google Workspace behind Okta, OAuth2

Okta is the identity provider in front of the Google account, so there is no password IMAP could take and app passwords are disabled by the admin. What works is OAuth2 over IMAP. You log in once in a browser, Okta does its MFA there, Google hands back a refresh token, and every sync after that is silent until the admin revokes it. symphonyd does this itself, no mbsync and no sasl plugins for this account.

Two things the admin controls. IMAP has to be enabled for the domain, under Gmail, End user access, and third party apps have to be allowed or allowlisted under Security, API controls. If either is off, IMAP is closed for that account and the login below fails with a message saying the app is blocked.

You need an OAuth client of your own. In a Google Cloud project, any account, make an OAuth client of type Desktop app, and under the consent screen add your school address as a test user. Put the id and secret beside the config, mode 600:

```
client_id = "....apps.googleusercontent.com"
client_secret = "...."
```

That file is ~/.config/symphony/google-oauth.toml. Then the account gets a user, auth = oauth, and a sync interval:

```
[[mail.accounts]]
name = "school"
maildir = "~/Mail/school"
user = "you@unlv.edu"
auth = "oauth"
sync = "10m"
fetch = 2000
```

user is also the From address on anything you send, and smtp overrides the SMTP host, smtp.gmail.com:465 when unset.

Authorize once, which opens the browser where Okta runs, and prints the url in case the browser does not open:

```
symphonyd mail authorize school
symphonyd mail sync school
```

The refresh token lands in ~/.config/symphony/tokens/school.json mode 600, treat it like a password. After that the daemon syncs the account every sync interval on its own and symphonyd mail sync forces one. fetch is how many of the newest messages it keeps locally, 2000 when unset, the sync reads the inbox and never writes to the server, a message that leaves the window on the server is removed locally, and flag changes on the server rename the local file. Personal Gmail works the same way with auth = oauth and your personal address as a test user, which is nicer than the app password, or with auth = password and pass_file pointing at the app password file.

## Proton Mail, Bridge

Proton is end to end encrypted and has no IMAP on the server. Proton Mail Bridge runs on your machine, logs in as you once with your password and a 2FA code, and exposes IMAP and SMTP on localhost with a password it generates. Proton requires an authenticator app before a security key can be added, so the YubiKey account always has a TOTP code to give Bridge. Bridge needs a paid plan and a secret store for its vault, pass or gnome-keyring.

```
paru -S protonmail-bridge
protonmail-bridge --cli
```

Inside the cli, login, then info prints the host, port, username, and the Bridge password. Put that password in a file and point mbsync at localhost. The port is 1143 with STARTTLS and Bridge's own certificate, which info also names.

```
IMAPAccount proton
Host 127.0.0.1
Port 1143
User you@proton.me
PassCmd "cat ~/.config/symphony/proton.pass"
TLSType STARTTLS
CertificateFile ~/.config/protonmail/bridge-v3/cert.pem

IMAPStore proton-remote
Account proton

MaildirStore proton-local
Path ~/Mail/proton/
Inbox ~/Mail/proton/inbox
SubFolders Verbatim

Channel proton
Far :proton-remote:"INBOX"
Near :proton-local:inbox
Create Near
Expunge Both
SyncState *
```

Bridge has to be running for the sync to work, so run it as a user service, protonmail-bridge --noninteractive, and sync with mbsync proton.

## Where this is going

The daemon syncs Google accounts itself now, OAuth2 or a password, and Proton stays behind Bridge since that is the only way in. mbsync still works for any account with no auth set, the daemon just reads whatever is in the maildir. Next the sync becomes pledges on the account's contract so authorize and sync show up in the runtime state the per account tags already carry, and the daemon pushes read and flagged back to the server.

# Status

Step 5 of 5, nvim embedded and drawn, every app is a page the plugin shows as a buffer, the daemon serves them over a socket, mail runs as a geas contract when built with make geas, and its classify pledge is a dusk body when built with make dusk. Next is more apps on the same shape, calendar, git, and discord, and multigrid so the chrome can sit beside the nvim windows.
