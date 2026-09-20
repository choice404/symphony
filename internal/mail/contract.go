//go:build geas

package mail

import (
	"context"
	"fmt"
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

// Contract shows mail through a signed Mailbox contract, the runtime's state is the service health
type Contract struct {
	// The runtime the module is loaded in
	rt *geas.Runtime
	// The Maildir root handed over as the maildir vow
	dir string
	// Guards the instance and the last list
	mu sync.Mutex
	// The signed instance, nil before the first render
	inst *geas.Instance
	// The last list, replaced whole on every render
	last []Message
}

/**
 * Bind
 * Binds the two host pledges of the module, list and open, once per process
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
	return rt.Bind(ContractName+".open", func(_ *geas.Instance, args []geas.Value) (geas.Value, error) {
		path, _ := args[0].(string)
		o, err := Open(path)
		if err != nil {
			return geas.Result{Value: geas.Sum{Tag: errUnreadable, Fields: []geas.Value{err.Error()}}}, nil
		}
		return geas.Result{Ok: true, Value: encodeOpened(o)}, nil
	})
}

/**
 * NewContract
 * Builds the contract backed mail view
 * @param rt {*geas.Runtime} - the runtime with the module loaded and the pledges bound
 * @param dir {string} - the Maildir root, empty when not configured
 * @return *Contract
 **/
func NewContract(rt *geas.Runtime, dir string) *Contract {
	return &Contract{rt: rt, dir: dir}
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
 * Returns the state and counts for the home page
 * @param ctx {context.Context} - the context
 * @return string
 **/
func (c *Contract) Summary(ctx context.Context) string {
	// Render to refresh the state
	msgs, err := c.list()
	if err != nil {
		return err.Error()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// The runtime's state and the counts
	return fmt.Sprintf("%s, %d messages, %d unread", strings.ToLower(c.inst.State().String()), len(msgs), Unread(msgs))
}

/**
 * Render
 * Lists the inbox through the contract
 * @param ctx {context.Context} - the context
 * @return view.Page, error
 **/
func (c *Contract) Render(ctx context.Context) (view.Page, error) {
	// List
	msgs, err := c.list()
	if err != nil {
		// A broken contract still gets a page, the error is the content
		return view.Page{
			Name:     ViewName,
			Title:    "mail",
			Lines:    []string{"mail " + err.Error(), "", "fix the config and press r"},
			Keys:     []string{"", "", ""},
			Filetype: "mail",
		}, nil
	}
	c.mu.Lock()
	state := strings.ToLower(c.inst.State().String())
	c.mu.Unlock()
	// The list with the state in the header
	return listPage(countHeader(msgs)+"  ["+state+"]", msgs), nil
}

/**
 * Act
 * Opens a message through the contract or refreshes the list
 * @param ctx {context.Context} - the context
 * @param action {string} - open or refresh
 * @param key {string} - the message id
 * @return view.Response, error
 **/
func (c *Contract) Act(ctx context.Context, action, key string) (view.Response, error) {
	// Dispatch on the action
	switch action {
	case "refresh":
		// Drop the instance so a fixed config gets a fresh sign
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
 * Fulfills open on the contract for one message
 * @param key {string} - the message id
 * @return view.Response, error
 **/
func (c *Contract) open(key string) (view.Response, error) {
	// Nothing to open on a header line
	if key == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	c.mu.Lock()
	inst := c.inst
	m, ok := find(&c.last, key)
	c.mu.Unlock()
	if inst == nil || !ok {
		return view.Fail("mail: message not in the current list, refresh"), nil
	}
	// Fulfill
	v, err := inst.Fulfill("open", m.Path)
	if err != nil {
		return view.Fail("mail: " + err.Error()), nil
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return view.Fail("mail " + describeError(r.Value)), nil
	}
	rec, _ := r.Value.(geas.Record)
	return view.Show(messagePage(decodeOpened(rec))), nil
}

/**
 * list
 * Signs on first use, then fulfills list and keeps the result, an Err becomes a Go error naming the variant
 * @return []Message, error
 **/
func (c *Contract) list() ([]Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Sign once and run the configured check, which breaks the contract when the vow is empty
	if c.inst == nil {
		inst, err := c.rt.Sign(ContractName, map[string]geas.Value{"maildir": c.dir})
		if err != nil {
			return nil, fmt.Errorf("sign: %w", err)
		}
		c.inst = inst
		if v, err := inst.Fulfill("configured"); err == nil {
			if r, _ := v.(geas.Result); !r.Ok {
				return nil, fmt.Errorf("%s: %s", strings.ToLower(inst.State().String()), describeError(r.Value))
			}
		}
	}
	// A broken contract stays broken until refresh re-signs
	if c.inst.State() == geas.Broken {
		return nil, fmt.Errorf("broken: %s", c.firstError())
	}
	// Fulfill list
	v, err := c.inst.Fulfill("list")
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return nil, fmt.Errorf("%s: %s", strings.ToLower(c.inst.State().String()), describeError(r.Value))
	}
	// Keep the list for open
	c.last = decodeMessages(r.Value)
	return c.last, nil
}

/**
 * firstError
 * Describes the first broken pledge's error
 * @return string
 **/
func (c *Contract) firstError() string {
	// The errors
	errs := c.inst.Errors()
	if len(errs) == 0 {
		return "no error recorded"
	}
	return errs[0].Pledge + " " + describeError(errs[0].Err)
}

/**
 * resign
 * Breaks the current instance so the next list signs a fresh one
 * @return void
 **/
func (c *Contract) resign() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inst != nil {
		_ = c.inst.Break()
		c.inst = nil
		c.last = nil
	}
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
