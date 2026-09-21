//go:build geas

package mail

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/geas"
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

// Contract is the backend that reads mail through one signed Mailbox contract per account, each runtime state is that account's health
type Contract struct {
	// The runtime the module is loaded in
	rt *geas.Runtime
	// Whether classify is bound to a real body, so labels are worth asking for
	labels bool
	// Guards the slots
	mu sync.Mutex
	// The signed instance per account name
	slots map[string]*slot
}

/**
 * Bind
 * Binds the host pledges of the module, list, open, and a classify that labels nothing until a dusk body replaces it
 * @param rt {*geas.Runtime} - the runtime with the mail module loaded
 * @return error
 **/
func Bind(rt *geas.Runtime) error {
	// list scans a folder of the maildir the vow names
	err := rt.Bind(ContractName+".list", func(inst *geas.Instance, args []geas.Value) (geas.Value, error) {
		dir, _ := inst.Vow("maildir")
		root, _ := dir.(string)
		folder, _ := args[0].(string)
		msgs, err := Scan(FolderDir(root, folder))
		if err != nil {
			return geas.Result{Value: geas.Sum{Tag: errMissing, Fields: []geas.Value{FolderDir(root, folder)}}}, nil
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
 * Builds the contract backend
 * @param rt {*geas.Runtime} - the runtime with the module loaded and the pledges bound
 * @param labels {bool} - whether classify has a real body
 * @return *Contract
 **/
func NewContract(rt *geas.Runtime, labels bool) *Contract {
	return &Contract{rt: rt, labels: labels, slots: map[string]*slot{}}
}

/**
 * List
 * Signs the account when needed, runs configured once, then fulfills list for the folder, the tag is the runtime state
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param folder {string} - the folder
 * @return []Message, string, error
 **/
func (c *Contract) List(ctx context.Context, a Account, folder string) ([]Message, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// The instance
	s, err := c.slotFor(a)
	if err != nil {
		return nil, "", err
	}
	// A broken contract stays broken until refresh re-signs
	if s.inst.State() == geas.Broken {
		return nil, "", fmt.Errorf("broken: %s", firstError(s.inst))
	}
	// Fulfill list
	s.lists++
	v, err := s.inst.Fulfill("list", folder)
	if err != nil {
		return nil, "", fmt.Errorf("list: %w", err)
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return nil, "", fmt.Errorf("%s: %s", strings.ToLower(s.inst.State().String()), describeError(r.Value))
	}
	// Tag
	msgs := decodeMessages(r.Value)
	for i := range msgs {
		msgs[i].Account = a.Name
		msgs[i].Folder = folder
	}
	return msgs, strings.ToLower(s.inst.State().String()), nil
}

/**
 * slotFor
 * Returns the account's instance, signing a fresh one when there is none, the maildir changed, or the old one served enough lists
 * @param a {Account} - the account
 * @return *slot, error
 **/
func (c *Contract) slotFor(a Account) (*slot, error) {
	// A changed maildir or a well used instance breaks the old one, the break is what frees every listed message
	s := c.slots[a.Name]
	if s != nil && (s.dir != a.Dir || s.lists >= listsPerInstance) {
		_ = s.inst.Break()
		delete(c.slots, a.Name)
		s = nil
	}
	if s != nil {
		return s, nil
	}
	// Sign and run the configured check, which breaks the contract when the vow is empty
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
	return s, nil
}

/**
 * Open
 * Fulfills open on the account's instance
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param m {Message} - the message
 * @return Opened, error
 **/
func (c *Contract) Open(ctx context.Context, a Account, m Message) (Opened, error) {
	c.mu.Lock()
	s := c.slots[a.Name]
	c.mu.Unlock()
	if s == nil {
		return Opened{}, fmt.Errorf("mail: %s is not signed, refresh", a.Name)
	}
	// Fulfill
	v, err := s.inst.Fulfill("open", m.Path)
	if err != nil {
		return Opened{}, fmt.Errorf("mail: %w", err)
	}
	r, _ := v.(geas.Result)
	if !r.Ok {
		return Opened{}, fmt.Errorf("mail %s", describeError(r.Value))
	}
	rec, _ := r.Value.(geas.Record)
	o := decodeOpened(rec)
	o.Account = m.Account
	o.Folder = m.Folder
	o.Path = m.Path
	o.MessageID = m.MessageID
	return o, nil
}

/**
 * Label
 * Fulfills classify for every message on the account's instance and returns a new list carrying the labels
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param msgs {[]Message} - the messages
 * @return []Message
 **/
func (c *Contract) Label(ctx context.Context, a Account, msgs []Message) []Message {
	// Nothing to ask without a real body
	if !c.labels {
		return msgs
	}
	c.mu.Lock()
	s := c.slots[a.Name]
	c.mu.Unlock()
	if s == nil {
		return msgs
	}
	// The labeled copies
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		v, err := s.inst.Fulfill("classify", m.From, m.Subject)
		if err == nil {
			if r, ok := v.(geas.Result); ok && r.Ok {
				m.Label = str(r.Value)
			}
		}
		out = append(out, m)
	}
	return out
}

/**
 * Refresh
 * Breaks every instance so the next list signs fresh ones
 * @return void
 **/
func (c *Contract) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, s := range c.slots {
		_ = s.inst.Break()
		delete(c.slots, name)
	}
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
			m.ID, m.Path, m.From, m.Subject, m.Date.Unix(), m.Seen, m.Flagged, m.Replied, m.MessageID,
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
		if !ok || len(rec.Fields) < 9 {
			continue
		}
		f := rec.Fields
		out = append(out, Message{
			ID:        str(f[0]),
			Path:      str(f[1]),
			From:      str(f[2]),
			Subject:   str(f[3]),
			Date:      unix(f[4]),
			Seen:      boolean(f[5]),
			Flagged:   boolean(f[6]),
			Replied:   boolean(f[7]),
			MessageID: str(f[8]),
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
	return geas.Record{Fields: []geas.Value{o.ID, o.From, o.To, o.Subject, o.Date.Unix(), o.Body, o.ReplyTo, o.References}}
}

/**
 * decodeOpened
 * Turns an Opened record back into an opened message
 * @param rec {geas.Record} - the record
 * @return Opened
 **/
func decodeOpened(rec geas.Record) Opened {
	// Pad short records so indexing is safe
	f := append(rec.Fields, make([]geas.Value, 8)...)
	return Opened{
		Message:    Message{ID: str(f[0]), From: str(f[1]), Subject: str(f[3]), Date: unix(f[4])},
		To:         str(f[2]),
		Body:       str(f[5]),
		ReplyTo:    str(f[6]),
		References: str(f[7]),
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
