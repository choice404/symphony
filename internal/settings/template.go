package settings

// Template is the config written when there is none yet, every section shown and commented out so the file documents itself
const Template = `# symphony config
# every section is optional, remove the leading # on the lines you want
# secrets never go here, they live in files beside this one with mode 600 or in env vars
# the daemon reads this file again after every write from inside symphony

# [mail]
# maildir = "~/mail"
# limit = 2000

# [[mail.accounts]]
# name = "personal"
# maildir = "~/mail/personal"
# user = "you@gmail.com"
# host = "imap.gmail.com"
# auth = "password"
# pass_file = "~/.config/symphony/personal.pass"
# fetch = "15m"
# sync = true
# smtp = "smtp.gmail.com:465"

# [calendar]
# days = 14

# [projects]
# roots = ["~/projects", "~/work"]

# [discord]
# token_file = "~/.config/symphony/discord.token"

# [browser]
# chrome = "/usr/bin/chromium"
# search = "http://127.0.0.1:8888/search?q="

# [editor]
# config = "symphony"   # or "own" to run your usual nvim config inside symphony

# [geas]
# contracts = "~/.local/share/symphony/contracts"
`
