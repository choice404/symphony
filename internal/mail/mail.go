package mail

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/view"
)

// ViewName is the name of the mail view
const ViewName = "mail"

// Services is what the view asks the daemon to do on the network, any of them nil when the build or the config cannot
type Services struct {
	// Sends a message from an account
	Send func(ctx context.Context, a Account, out Outgoing) error
	// Moves a message to the trash on the server and removes the local file
	Trash func(ctx context.Context, a Account, m Message) error
	// Asks for a sync of an account soon, after a send so the sent folder catches up
	SyncNow func(name string)
	// Judges which inbox messages are spam, a probability per message key, nil when no judge is configured
	Spam func(ctx context.Context, a Account, msgs []Message) (map[string]float64, error)
}

// Mail is the mail app, one view over every account and folder
type Mail struct {
	// Returns the settings as configured right now
	src Source
	// Where messages come from
	back Backend
	// What the daemon does on the network
	svc Services
	// Guards the lists, drafts, and marks
	mu sync.Mutex
	// The last list per page name, so open finds a key
	lists map[string][]Message
	// The open drafts by number
	drafts map[int]Draft
	// The next draft number
	next int
	// The spam marks by message key, for the review page
	marks map[string]bool
}

/**
 * New
 * Builds the mail view
 * @param src {Source} - the settings, read on every render
 * @param back {Backend} - where messages come from
 * @param svc {Services} - what the daemon does on the network
 * @return *Mail
 **/
func New(src Source, back Backend, svc Services) *Mail {
	return &Mail{src: src, back: back, svc: svc, lists: map[string][]Message{}, drafts: map[int]Draft{}, next: 1, marks: map[string]bool{}}
}

/**
 * Name
 * Returns mail
 * @return string
 **/
func (m *Mail) Name() string {
	return ViewName
}

/**
 * Entries
 * Builds the home page entries, one per account then one for all of them when there are several
 * @return []view.Entry
 **/
func (m *Mail) Entries() []view.Entry {
	// The accounts as configured right now
	accs := m.src().Accounts
	out := make([]view.Entry, 0, len(accs)+1)
	for _, a := range accs {
		a := a
		out = append(out, view.Entry{
			Name:    pageName(a.Name, FolderInbox),
			Label:   "mail " + a.Name,
			Summary: func(ctx context.Context) string { return m.summary(ctx, a) },
		})
	}
	if len(accs) > 1 {
		out = append(out, view.Entry{Name: ViewName, Label: "mail all", Summary: func(ctx context.Context) string { return m.summaryAll(ctx) }})
	}
	if len(accs) == 0 {
		out = append(out, view.Entry{Name: ViewName, Label: "mail", Summary: func(context.Context) string { return "not configured" }})
	}
	return out
}

/**
 * summary
 * Returns one account's inbox line for home, its health and its counts
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @return string
 **/
func (m *Mail) summary(ctx context.Context, a Account) string {
	// A missing directory is unconfigured
	if a.Dir == "" {
		return "not configured"
	}
	msgs, tag, err := m.list(ctx, a, FolderInbox)
	if err != nil {
		return err.Error()
	}
	if tag != "" {
		tag += ", "
	}
	return fmt.Sprintf("%s%d messages, %d unread", tag, len(msgs), Unread(msgs))
}

/**
 * summaryAll
 * Returns the merged inbox line for home
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (m *Mail) summaryAll(ctx context.Context) string {
	// The merged list and the tags
	msgs, tags, _ := m.merged(ctx, m.src().Accounts, FolderInbox)
	return fmt.Sprintf("%d messages, %d unread", len(msgs), Unread(msgs)) + notes(tags)
}

/**
 * Render
 * Shows the merged inbox
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (m *Mail) Render(ctx context.Context) (view.Page, error) {
	return m.RenderPath(ctx, AllAccounts+"/"+FolderInbox)
}

/**
 * RenderPath
 * Shows a list, account and folder, or a message, account, folder, and id
 * @param ctx {context.Context} - the context
 * @param path {string} - the path below mail
 * @return view.Page, error
 **/
func (m *Mail) RenderPath(ctx context.Context, path string) (view.Page, error) {
	// The parts
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return view.Page{}, fmt.Errorf("mail: a page is account/folder, not %q", path)
	}
	account, folder := parts[0], parts[1]
	// A compose page is made by an action, not by name
	if folder == "compose" {
		return view.Page{}, fmt.Errorf("mail: a compose page is opened with c, R, or F")
	}
	// A message
	if len(parts) == 3 {
		return m.message(ctx, strings.Join(parts, "/"))
	}
	// A list
	return m.listPage(ctx, account, folder)
}

/**
 * listPage
 * Builds the list page of an account and folder, or of every account
 * @param ctx {context.Context} - the context
 * @param account {string} - the account name or all
 * @param folder {string} - the folder
 * @return view.Page, error
 **/
func (m *Mail) listPage(ctx context.Context, account, folder string) (view.Page, error) {
	// The settings
	set := m.src()
	accs := set.Accounts
	if len(accs) == 0 {
		return unconfiguredPage, nil
	}
	// The spam page is its own thing
	if folder == FolderSpam {
		return m.spamPage(ctx, account)
	}
	// One account or all of them
	var msgs []Message
	tags := map[string]string{}
	multi := false
	if account == AllAccounts {
		msgs, tags, _ = m.merged(ctx, accs, folder)
		multi = true
	} else {
		a, ok := findAccount(accs, account)
		if !ok {
			return view.Page{}, fmt.Errorf("mail: no account named %q", account)
		}
		got, tag, err := m.list(ctx, a, folder)
		if err != nil {
			return errorPage(account, folder, err.Error()), nil
		}
		msgs = got
		if tag != "" {
			tags[a.Name] = tag
		}
	}
	// Cut, label, and remember
	msgs = newest(msgs, set.Limit)
	msgs = m.labelAll(ctx, accs, msgs)
	p := listPage(account, folder, countHeader(account, folder, msgs)+notes(tags), msgs, multi)
	m.remember(p.Name, msgs)
	return p, nil
}

/**
 * list
 * Reads one folder of one account through the backend
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param folder {string} - the folder
 * @return []Message, string, error
 **/
func (m *Mail) list(ctx context.Context, a Account, folder string) ([]Message, string, error) {
	// A missing directory is unconfigured
	if a.Dir == "" {
		return nil, "", fmt.Errorf("not configured")
	}
	msgs, tag, err := m.back.List(ctx, a, folder)
	if err != nil {
		return nil, "", err
	}
	sortNewest(msgs)
	return msgs, tag, nil
}

/**
 * merged
 * Reads one folder of every account into one list newest first, with a tag per account, its health or its error
 * @param ctx {context.Context} - the context
 * @param accs {[]Account} - the accounts
 * @param folder {string} - the folder
 * @return []Message, map[string]string, bool
 **/
func (m *Mail) merged(ctx context.Context, accs []Account, folder string) ([]Message, map[string]string, bool) {
	// Every account
	all := make([]Message, 0, 64)
	tags := map[string]string{}
	for _, a := range accs {
		msgs, tag, err := m.list(ctx, a, folder)
		if err != nil {
			tags[a.Name] = err.Error()
			continue
		}
		if tag != "" {
			tags[a.Name] = tag
		}
		all = append(all, msgs...)
	}
	sortNewest(all)
	return all, tags, len(accs) > 1
}

/**
 * labelAll
 * Runs the backend's classifier per account over a merged list
 * @param ctx {context.Context} - the context
 * @param accs {[]Account} - the accounts
 * @param msgs {[]Message} - the messages
 * @return []Message
 **/
func (m *Mail) labelAll(ctx context.Context, accs []Account, msgs []Message) []Message {
	// Group by account, label, then put back in order
	byAccount := map[string][]Message{}
	for _, msg := range msgs {
		byAccount[msg.Account] = append(byAccount[msg.Account], msg)
	}
	labeled := map[string]Message{}
	for name, group := range byAccount {
		a, ok := findAccount(accs, name)
		if !ok {
			continue
		}
		for _, l := range m.back.Label(ctx, a, group) {
			labeled[l.Key()] = l
		}
	}
	out := make([]Message, 0, len(msgs))
	for _, msg := range msgs {
		if l, ok := labeled[msg.Key()]; ok {
			out = append(out, l)
		} else {
			out = append(out, msg)
		}
	}
	return out
}

/**
 * remember
 * Keeps a page's list so open finds a key later
 * @param page {string} - the page name
 * @param msgs {[]Message} - the list
 * @return void
 **/
func (m *Mail) remember(page string, msgs []Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists[page] = msgs
}

/**
 * lookup
 * Finds a message by key in any remembered list
 * @param key {string} - the message key
 * @return Message, bool
 **/
func (m *Mail) lookup(key string) (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msgs := range m.lists {
		if msg, ok := findKey(msgs, key); ok {
			return msg, true
		}
	}
	return Message{}, false
}

/**
 * message
 * Builds the page of one message by key
 * @param ctx {context.Context} - the context
 * @param key {string} - account, folder, and id
 * @return view.Page, error
 **/
func (m *Mail) message(ctx context.Context, key string) (view.Page, error) {
	// The message and its account
	o, err := m.open(ctx, key)
	if err != nil {
		return view.Page{}, err
	}
	return messagePage(o), nil
}

/**
 * open
 * Reads a message by key through the backend
 * @param ctx {context.Context} - the context
 * @param key {string} - the message key
 * @return Opened, error
 **/
func (m *Mail) open(ctx context.Context, key string) (Opened, error) {
	// The message must be in a list that was shown
	msg, ok := m.lookup(key)
	if !ok {
		return Opened{}, fmt.Errorf("mail: message not in a current list, refresh")
	}
	a, ok := findAccount(m.src().Accounts, msg.Account)
	if !ok {
		return Opened{}, fmt.Errorf("mail: no account named %q", msg.Account)
	}
	return m.back.Open(ctx, a, msg)
}

/**
 * Act
 * Runs an action from a mail page
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the action
 * @return view.Response, error
 **/
func (m *Mail) Act(ctx context.Context, a view.Action) (view.Response, error) {
	// Where the action came from
	_, path := view.Split(a.Page)
	parts := strings.Split(path, "/")
	account, folder := AllAccounts, FolderInbox
	if len(parts) >= 2 {
		account, folder = parts[0], parts[1]
	}
	// Dispatch on the action
	switch a.Name {
	case "open":
		if a.Key == "" {
			return view.Response{Kind: view.KindNone}, nil
		}
		return m.show(m.message(ctx, a.Key))
	case "refresh":
		m.back.Refresh()
		return m.show(m.listPage(ctx, account, folder))
	case "next", "prev":
		return m.show(m.listPage(ctx, m.neighbor(account, a.Name == "next"), folder))
	case "folder":
		return m.show(m.listPage(ctx, account, a.Key))
	case "all":
		return m.show(m.listPage(ctx, AllAccounts, folder))
	case "compose":
		return m.compose(account)
	case "reply", "forward":
		return m.replyOrForward(ctx, a.Name, a.Key)
	case "send":
		return m.send(ctx, a)
	case "trash":
		return m.trash(ctx, a.Key, account, folder)
	case "mark":
		return m.mark(ctx, a.Key, account)
	case "purge":
		return m.purge(ctx, account)
	}
	return view.Fail("mail: unknown action " + a.Name), nil
}

/**
 * show
 * Turns a page or its error into a response
 * @param p {view.Page} - the page
 * @param err {error} - the error
 * @return view.Response, error
 **/
func (m *Mail) show(p view.Page, err error) (view.Response, error) {
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	return view.Show(p), nil
}

/**
 * neighbor
 * Returns the account after or before one in the cycle all, then each account in config order
 * @param account {string} - the current account or all
 * @param forward {bool} - whether to step forward
 * @return string
 **/
func (m *Mail) neighbor(account string, forward bool) string {
	// The cycle
	names := []string{AllAccounts}
	for _, a := range m.src().Accounts {
		names = append(names, a.Name)
	}
	// Where we are
	at := 0
	for i, n := range names {
		if n == account {
			at = i
		}
	}
	// Step
	if forward {
		return names[(at+1)%len(names)]
	}
	return names[(at-1+len(names))%len(names)]
}

/**
 * accountFor
 * Picks the account a compose page belongs to, the named one or the first when composing from all
 * @param account {string} - the account name or all
 * @return Account, error
 **/
func (m *Mail) accountFor(account string) (Account, error) {
	accs := m.src().Accounts
	if len(accs) == 0 {
		return Account{}, fmt.Errorf("mail: no accounts configured")
	}
	if account == AllAccounts {
		return accs[0], nil
	}
	a, ok := findAccount(accs, account)
	if !ok {
		return Account{}, fmt.Errorf("mail: no account named %q", account)
	}
	return a, nil
}

/**
 * compose
 * Opens an empty compose page for an account
 * @param account {string} - the account name or all
 * @return view.Response, error
 **/
func (m *Mail) compose(account string) (view.Response, error) {
	// The account
	a, err := m.accountFor(account)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	if a.User == "" {
		return view.Fail("mail: set user on " + a.Name + " before composing"), nil
	}
	// A new draft
	n := m.newDraft(Draft{Account: a.Name})
	return view.Show(composePage(n, a, "", "", "")), nil
}

/**
 * newDraft
 * Records a draft and returns its number
 * @param d {Draft} - the draft
 * @return int
 **/
func (m *Mail) newDraft(d Draft) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.next
	m.next++
	m.drafts[n] = d
	return n
}

/**
 * replyOrForward
 * Opens a compose page prefilled from a message
 * @param ctx {context.Context} - the context
 * @param what {string} - reply or forward
 * @param key {string} - the message key
 * @return view.Response, error
 **/
func (m *Mail) replyOrForward(ctx context.Context, what, key string) (view.Response, error) {
	// The message in full
	if key == "" {
		return view.Fail("mail: nothing under the cursor to " + what + " to"), nil
	}
	o, err := m.open(ctx, key)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	a, err := m.accountFor(o.Account)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	if a.User == "" {
		return view.Fail("mail: set user on " + a.Name + " before composing"), nil
	}
	// A reply goes back to Reply-To or From with the thread headers, a forward goes to nobody yet
	if what == "reply" {
		to := o.ReplyTo
		if to == "" {
			to = o.From
		}
		n := m.newDraft(Draft{Account: a.Name, InReplyTo: o.MessageID, References: strings.TrimSpace(o.References + " " + o.MessageID)})
		return view.Show(composePage(n, a, to, replySubject(o.Subject), replyBody(o))), nil
	}
	n := m.newDraft(Draft{Account: a.Name})
	return view.Show(composePage(n, a, "", forwardSubject(o.Subject), forwardBody(o))), nil
}

/**
 * send
 * Parses a compose buffer and sends it through the daemon
 * @param ctx {context.Context} - the context
 * @param a {view.Action} - the send action with the buffer text
 * @return view.Response, error
 **/
func (m *Mail) send(ctx context.Context, a view.Action) (view.Response, error) {
	// The draft behind the page
	n, err := strconv.Atoi(a.Key)
	if err != nil {
		return view.Fail("mail: send works from a compose page"), nil
	}
	m.mu.Lock()
	d, ok := m.drafts[n]
	m.mu.Unlock()
	if !ok {
		return view.Fail("mail: this draft is not open any more, compose again"), nil
	}
	if m.svc.Send == nil {
		return view.Fail("mail: sending is not available, set auth on the account"), nil
	}
	acc, err := m.accountFor(d.Account)
	if err != nil {
		return view.Fail(err.Error()), nil
	}
	// Parse and send
	out, err := parseCompose(a.Body, d, time.Now())
	if err != nil {
		return view.Fail("mail: " + err.Error()), nil
	}
	if err := m.svc.Send(ctx, acc, out); err != nil {
		return view.Fail("mail: send failed, " + err.Error()), nil
	}
	// Forget the draft, close its buffer, and catch the sent folder up
	m.mu.Lock()
	delete(m.drafts, n)
	m.mu.Unlock()
	if m.svc.SyncNow != nil {
		m.svc.SyncNow(acc.Name)
	}
	resp := view.Notify("mail: sent " + strconv.Quote(out.Subject) + " to " + strings.Join(out.Recipients, ", "))
	resp.Close = a.Page
	return resp, nil
}

/**
 * trash
 * Moves a message to the trash through the daemon, then shows the list the action came from and closes the message page when it came from one
 * @param ctx {context.Context} - the context
 * @param key {string} - the message key
 * @param account {string} - the account of the page the action came from
 * @param folder {string} - the folder of that page
 * @return view.Response, error
 **/
func (m *Mail) trash(ctx context.Context, key, account, folder string) (view.Response, error) {
	// The message
	if key == "" {
		return view.Fail("mail: nothing under the cursor to trash"), nil
	}
	msg, ok := m.lookup(key)
	if !ok {
		return view.Fail("mail: message not in a current list, refresh"), nil
	}
	if m.svc.Trash == nil {
		return view.Fail("mail: trash is not available, set auth on the account"), nil
	}
	a, ok := findAccount(m.src().Accounts, msg.Account)
	if !ok {
		return view.Fail("mail: no account named " + msg.Account), nil
	}
	if err := m.svc.Trash(ctx, a, msg); err != nil {
		return view.Fail("mail: trash failed, " + err.Error()), nil
	}
	// The list again, and the message page closed if that is where we were
	resp, err := m.show(m.listPage(ctx, account, folder))
	if err == nil && resp.Kind == view.KindPage {
		resp.Close = ViewName + "/" + key
		resp.Text = "mail: trashed " + strconv.Quote(msg.Subject)
	}
	return resp, err
}
