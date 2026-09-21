//go:build geas

package mail

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/geas"
	"github.com/choice404/symphony/internal/view"
)

// ContractName is the contract the module declares
const ContractName = "Mailbox"

// The variant indexes of MailError in declaration order
const (
	errNotConfigured = 0
	errMissing       = 1
	errUnreadable    = 2
)

// listsPerInstance is how many list fulfillments an instance serves before it is re-signed, every fulfillment copies its result onto the instance and only a break frees it
const listsPerInstance = 20

// slot is one signed Mailbox instance for one account
type slot struct {
	// The signed instance
	inst *geas.Instance
	// The maildir it was signed with, a change re-signs
	dir string
	// How many lists this instance has served
	lists int
}

// Contract shows mail through one signed Mailbox contract per account, each runtime state is that account's health
type Contract struct {
	// The runtime the module is loaded in
	rt *geas.Runtime
	// Returns the accounts as configured right now
	src Source
	// Whether classify is bound, so every listed message gets a label
	labels bool
	// Guards the slots and the last list
	mu sync.Mutex
	// The signed instance per account name
	slots map[string]*slot
	// The last merged list, replaced whole on every render
	last []Message
}

/**
 * Bind
 * Binds the host pledges of the module, list, open, and a classify that labels nothing until a dusk body replaces it
 * @param rt {*geas.Runtime} - the runtime with the mail module loaded
 * @return error
 **/
func Bind(rt *geas.Runtime) error {
	// list scans the maildir the vow names
	err := rt.Bind(ContractName+".list", func(inst *geas.Instance, _ []geas.Value) (geas.Value, error) {
		dir, _ := inst.Vow("maildir")
		root, _ := dir.(string)
		msgs, err := Scan(root)
		if err != nil {
			return geas.Result{Value: geas.Sum{Tag: errMissing, Fields: []geas.Value{root}}}, nil
		}
		return geas.Result{Ok: true, Value: encodeMessages(msgs)}, nil
	})
	if err != nil {
		return err
	}
	// open reads one file
	err = rt.Bind(ContractName+".open", func(_ *geas.Instance, args []geas.Value) (geas.Value, error) {
		path, _ := args[0].(string)
		o, err := Open(path)
		if err != nil {
			return geas.Result{Value: geas.Sum{Tag: errUnreadable, Fields: []geas.Value{err.Error()}}}, nil
		}
		return geas.Result{Ok: true, Value: encodeOpened(o)}, nil
	})
	if err != nil {
		return err
	}
	// classify labels nothing until BindDusk replaces it, so the contract can always sign
	return rt.Bind(ContractName+".classify", func(_ *geas.Instance, _ []geas.Value) (geas.Value, error) {
		return geas.Result{Ok: true, Value: ""}, nil
	})
}

/**
 * NewContract
 * Builds the contract backed mail view
 * @param rt {*geas.Runtime} - the runtime with the module loaded and the pledges bound
 * @param src {Source} - returns the accounts, called on every render so a config edit re-signs without a restart
 * @param labels {bool} - whether classify is bound and every message should carry a label
 * @return *Contract
 **/
func NewContract(rt *geas.Runtime, src Source, labels bool) *Contract {
	return &Contract{rt: rt, src: src, labels: labels, slots: map[string]*slot{}}
}

/**
 * Name
 * Returns mail
 * @return string
 **/
func (c *Contract) Name() string {
	return ViewName
}

/**
 * Summary
 * Returns the state and counts for the home page, one account as before and several as a tag each
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (c *Contract) Summary(ctx context.Context) string {
	// List everything
	msgs, states, errs, single := c.list()
	// A lone account reads as its state and the counts, or its error alone
	if single {
		if len(errs) == 1 {
			for _, e := range errs {
				return e
			}
		}
		for _, st := range states {
			return fmt.Sprintf("%s, %d messages, %d unread", st, len(msgs), Unread(msgs))
		}
	}
	// Several accounts read as the counts then a tag per account
	tags := map[string]string{}
	for name, st := range states {
		tags[name] = st
	}
	for name, e := range errs {
		tags[name] = e
	}
	if len(tags) == 0 {
		return "not configured"
	}
	return fmt.Sprintf("%d messages, %d unread", len(msgs), Unread(msgs)) + notes(tags)
}

/**
 * Render
 * Lists every account's inbox through its contract
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (c *Contract) Render(ctx context.Context) (view.Page, error) {
	// List everything
	msgs, states, errs, single := c.list()
	// Nothing configured at all
	if len(states) == 0 && len(errs) == 0 {
		return unconfiguredPage, nil
	}
	// A lone broken account gets the error as the page
	if single && len(errs) == 1 {
		for _, e := range errs {
			return view.Page{
				Name:     ViewName,
				Title:    "mail",
				Lines:    []string{"mail " + e, "", "fix the config and press r"},
				Keys:     []string{"", "", ""},
				Filetype: "mail",
			}, nil
		}
	}
	// The header carries the counts then a tag per account, its state or its error
	tags := map[string]string{}
	for name, st := range states {
		tags[name] = st
	}
	for name, e := range errs {
		tags[name] = e
	}
	return listPage(countHeader(msgs)+notes(tags), msgs, !single), nil
}

/**
 * Act
 * Opens a message through its account's contract or refreshes the list
 * @param ctx {context.Context} - the context
 * @param action {string} - open or refresh
 * @param key {string} - the message key
 * @return view.Response, error
 **/
func (c *Contract) Act(ctx context.Context, action, key string) (view.Response, error) {
	// Dispatch on the action
	switch action {
	case "refresh":
		// Drop every instance so a fixed config gets a fresh sign
		c.resign()
		p, err := c.Render(ctx)
		if err != nil {
			return view.Fail(err.Error()), nil
		}
		return view.Show(p), nil
	case "open":
		return c.open(key)
	}
	return view.Fail("mail: unknown action " + action), nil
}

/**
 * open
 * Fulfills open on the account's contract for one message
 * @param key {string} - the message key
 * @return view.Response, error
 **/
func (c *Contract) open(key string) (view.Response, error) {
	// Nothing to open on a header line
	if key == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	c.mu.Lock()
	m, ok := findKey(c.last, key)
	s := c.slots[m.Account]
	c.mu.Unlock()
	if !ok || s == nil {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	// Fulfill
	v, err := s.inst.Fulfill("open", m.Path)
	if err != nil {
		return view.Fail("mail: " + err.Error()), nil
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return view.Fail("mail " + describeError(r.Value)), nil
	}
	rec, _ := r.Value.(geas.Record)
	o := decodeOpened(rec)
	o.Account = m.Account
	return view.Show(messagePage(o)), nil
}

/**
 * list
 * Signs each account on first use or after its maildir changed, fulfills list on each, and merges the results newest first
 * @return []Message, map[string]string, map[string]string, bool
 **/
func (c *Contract) list() (msgs []Message, states, errs map[string]string, single bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// The accounts as configured right now, an empty maildir still gets an instance so the contract reports not configured
	set := c.src()
	accs := set.Accounts
	single = len(accs) <= 1
	states = map[string]string{}
	errs = map[string]string{}
	// Drop instances of accounts that are gone
	seen := map[string]bool{}
	for _, a := range accs {
		seen[a.Name] = true
	}
	for name, s := range c.slots {
		if !seen[name] {
			_ = s.inst.Break()
			delete(c.slots, name)
		}
	}
	// Loop over every account
	all := make([]Message, 0, 64)
	for _, a := range accs {
		got, err := c.listOne(a)
		if err != nil {
			errs[a.Name] = err.Error()
			continue
		}
		states[a.Name] = strings.ToLower(c.slots[a.Name].inst.State().String())
		all = append(all, got...)
	}
	// Newest first across accounts, cut to the limit, then labeled, so only shown messages cost a classify
	sort.SliceStable(all, func(i, j int) bool {
		if !all[i].Date.Equal(all[j].Date) {
			return all[i].Date.After(all[j].Date)
		}
		return all[i].Path < all[j].Path
	})
	all = newest(all, set.Limit)
	if c.labels {
		all = c.classify(all)
	}
	// Keep the list for open
	c.last = all
	return all, states, errs, single
}

/**
 * listOne
 * Signs one account when needed, runs configured once, then fulfills list, tagging every message with the account
 * @param a {Account} - the account
 * @return []Message, error
 **/
func (c *Contract) listOne(a Account) ([]Message, error) {
	// A changed maildir or a well used instance breaks the old one, the break is what frees every listed message
	s := c.slots[a.Name]
	if s != nil && (s.dir != a.Dir || s.lists >= listsPerInstance) {
		_ = s.inst.Break()
		delete(c.slots, a.Name)
		s = nil
	}
	// Sign once and run the configured check, which breaks the contract when the vow is empty
	if s == nil {
		inst, err := c.rt.Sign(ContractName, map[string]geas.Value{"maildir": a.Dir})
		if err != nil {
			return nil, fmt.Errorf("sign: %w", err)
		}
		s = &slot{inst: inst, dir: a.Dir}
		c.slots[a.Name] = s
		if v, err := inst.Fulfill("configured"); err == nil {
			if r, _ := v.(geas.Result); !r.Ok {
				return nil, fmt.Errorf("%s: %s", strings.ToLower(inst.State().String()), describeError(r.Value))
			}
		}
	}
	// A broken contract stays broken until refresh re-signs
	if s.inst.State() == geas.Broken {
		return nil, fmt.Errorf("broken: %s", firstError(s.inst))
	}
	// Fulfill list
	s.lists++
	v, err := s.inst.Fulfill("list")
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return nil, fmt.Errorf("%s: %s", strings.ToLower(s.inst.State().String()), describeError(r.Value))
	}
	// Tag
	msgs := decodeMessages(r.Value)
	for i := range msgs {
		msgs[i].Account = a.Name
	}
	return msgs, nil
}

/**
 * classify
 * Fulfills classify for every message on its account's instance and returns a new list carrying the labels
 * @param msgs {[]Message} - the messages
 * @return []Message
 **/
func (c *Contract) classify(msgs []Message) []Message {
	// The labeled copies
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		// Ask the plugin through the account's instance
		if s := c.slots[m.Account]; s != nil {
			v, err := s.inst.Fulfill("classify", m.From, m.Subject)
			if err == nil {
				if r, ok := v.(geas.Result); ok && r.Ok {
					m.Label = str(r.Value)
				}
			}
		}
		out = append(out, m)
	}
	return out
}

/**
 * firstError
 * Describes the first broken pledge's error on an instance
 * @param inst {*geas.Instance} - the instance
 * @return string
 **/
func firstError(inst *geas.Instance) string {
	// The errors
	errs := inst.Errors()
	if len(errs) == 0 {
		return "no error recorded"
	}
	return errs[0].Pledge + " " + describeError(errs[0].Err)
}

/**
 * resign
 * Breaks every instance so the next list signs fresh ones
 * @return void
 **/
func (c *Contract) resign() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, s := range c.slots {
		_ = s.inst.Break()
		delete(c.slots, name)
	}
	c.last = nil
}

/**
 * describeError
 * Spells a MailError variant
 * @param v {geas.Value} - the Err payload
 * @return string
 **/
func describeError(v geas.Value) string {
	// The variant
	s, ok := v.(geas.Sum)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	// The one payload field when there is one
	detail := ""
	if len(s.Fields) > 0 {
		detail, _ = s.Fields[0].(string)
	}
	// Spell it
	switch s.Tag {
	case errNotConfigured:
		return "not configured, set maildir under [mail] in ~/.config/symphony/config.toml"
	case errMissing:
		return "maildir missing at " + detail
	case errUnreadable:
		return "unreadable, " + detail
	}
	return fmt.Sprintf("error %d", s.Tag)
}

/**
 * encodeMessages
 * Turns messages into a list of Message records in the contract's field order
 * @param msgs {[]Message} - the messages
 * @return geas.Value
 **/
func encodeMessages(msgs []Message) geas.Value {
	// The records
	items := make([]geas.Value, 0, len(msgs))
	for _, m := range msgs {
		items = append(items, geas.Record{Fields: []geas.Value{
			m.ID, m.Path, m.From, m.Subject, m.Date.Unix(), m.Seen, m.Flagged, m.Replied,
		}})
	}
	return geas.List{Elem: geas.TagRecord, Items: items}
}

/**
 * decodeMessages
 * Turns a list of Message records back into messages
 * @param v {geas.Value} - the list
 * @return []Message
 **/
func decodeMessages(v geas.Value) []Message {
	// The list
	list, _ := v.(geas.List)
	out := make([]Message, 0, len(list.Items))
	for _, it := range list.Items {
		rec, ok := it.(geas.Record)
		if !ok || len(rec.Fields) < 8 {
			continue
		}
		f := rec.Fields
		out = append(out, Message{
			ID:      str(f[0]),
			Path:    str(f[1]),
			From:    str(f[2]),
			Subject: str(f[3]),
			Date:    unix(f[4]),
			Seen:    boolean(f[5]),
			Flagged: boolean(f[6]),
			Replied: boolean(f[7]),
		})
	}
	return out
}

/**
 * encodeOpened
 * Turns an opened message into an Opened record
 * @param o {Opened} - the message
 * @return geas.Value
 **/
func encodeOpened(o Opened) geas.Value {
	return geas.Record{Fields: []geas.Value{o.ID, o.From, o.To, o.Subject, o.Date.Unix(), o.Body}}
}

/**
 * decodeOpened
 * Turns an Opened record back into an opened message
 * @param rec {geas.Record} - the record
 * @return Opened
 **/
func decodeOpened(rec geas.Record) Opened {
	// Pad short records so indexing is safe
	f := append(rec.Fields, make([]geas.Value, 6)...)
	return Opened{
		Message: Message{ID: str(f[0]), From: str(f[1]), Subject: str(f[3]), Date: unix(f[4])},
		To:      str(f[2]),
		Body:    str(f[5]),
	}
}

// str reads a string value or empty
func str(v geas.Value) string {
	s, _ := v.(string)
	return s
}

// boolean reads a bool value or false
func boolean(v geas.Value) bool {
	b, _ := v.(bool)
	return b
}

// unix reads an int value as a time, zero when missing
func unix(v geas.Value) time.Time {
	n, ok := v.(int64)
	if !ok || n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
