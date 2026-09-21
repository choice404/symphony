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

[geas]
contracts = "~/.local/share/symphony/contracts"
```

With more than one account the inbox is one list across all of them newest first with an account column, and the header carries a tag per account, its contract state or its error, so one broken account never hides the others. Home shows the same tags. Each account is its own signed Mailbox contract, so the health you see is per account.

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
```

Then mbsync personal, and the maildir in the symphony config is ~/Mail/personal/inbox. Gmail also takes OAuth2 the same way the school account does below, which is what symphony will use itself once it syncs on its own.

## University Google Workspace behind Okta, OAuth2

Okta is the identity provider in front of the Google account, so there is no password IMAP could take and an app password is usually disabled by the admin. What works is OAuth2 over IMAP, XOAUTH2. You log in once in a browser, Okta does its MFA there, Google hands back a refresh token, and every sync after that is silent until the admin revokes it.

Two things the admin controls. IMAP has to be enabled for the domain, under Gmail, End user access, and third party apps have to be allowed or allowlisted under Security, API controls. If either is off, IMAP is closed for that account and only the Gmail API works, which is the native path below.

You need an OAuth client of your own. In a Google Cloud project, any account, make an OAuth client of type Desktop app, add the mail scope https://mail.google.com/, and add your school address as a test user. mbsync cannot speak XOAUTH2 by itself, it needs the cyrus-sasl-xoauth2 plugin, on Arch cyrus-sasl-xoauth2-git from the AUR, and a helper that stores the refresh token and prints a fresh access token. The mutt one works, mutt_oauth2.py, or oama from the AUR. With mutt_oauth2.py, put your client id and secret into the script's Google section, then authorize once, which opens the browser where Okta runs.

```
mutt_oauth2.py --authorize ~/.config/symphony/school.tokens
```

```
IMAPAccount school
Host imap.gmail.com
User you@university.edu
AuthMechs XOAUTH2
PassCmd "mutt_oauth2.py ~/.config/symphony/school.tokens"
TLSType IMAPS

IMAPStore school-remote
Account school

MaildirStore school-local
Path ~/Mail/school/
Inbox ~/Mail/school/inbox
SubFolders Verbatim

Channel school
Far :school-remote:"INBOX"
Near :school-local:inbox
Create Near
Expunge Both
SyncState *
```

The token file holds a refresh token that opens your mail, keep it mode 600 and treat it like a password.

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

mbsync is the stopgap. The daemon was always meant to own the network, and Go has IMAP with XOAUTH2 and the OAuth2 device code flow, so the plan is that symphonyd runs the OAuth2 login itself, prints the URL or opens the browser, you do Okta or the YubiKey there once, it stores the refresh token under ~/.config/symphony mode 600 or in the system keyring, and syncs IMAP into the Maildir on its own. Same for Gmail, and Proton stays behind Bridge since that is the only way in. No sidecars, one binary, and each account becomes its own contract with authorize and sync as pledges and the runtime state as its health, which is what the per account tags already show.

# Status

Step 5 of 5, nvim embedded and drawn, every app is a page the plugin shows as a buffer, the daemon serves them over a socket, mail runs as a geas contract when built with make geas, and its classify pledge is a dusk body when built with make dusk. Next is more apps on the same shape, calendar, git, and discord, and multigrid so the chrome can sit beside the nvim windows.
