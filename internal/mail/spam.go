package mail

import (
	"context"
	"fmt"
	"sort"

	"github.com/choice404/symphony/internal/view"
)

// spamHint is the second line of the spam page
const spamHint = "  d mark or unmark   x trash every marked message   <CR> open   gi back to inbox   r refresh"

// spamFloor is the lowest probability the review page lists
const spamFloor = 0.5

/**
 * spamPage
 * Lists the inbox messages the judge calls spam, most likely first, with a mark column
 * @param ctx {context.Context} - the context
 * @param account {string} - the account name or all
 * @return view.Page, error
 **/
func (m *Mail) spamPage(ctx context.Context, account string) (view.Page, error) {
	// Nothing to judge with
	if m.svc.Spam == nil {
		return errorPage(account, FolderSpam, "spam review needs a judge, set the TypeSafe key in the config"), nil
	}
	// The accounts in play
	set := m.src()
	accs := set.Accounts
	if account != AllAccounts {
		a, ok := findAccount(accs, account)
		if !ok {
			return view.Page{}, fmt.Errorf("mail: no account named %q", account)
		}
		accs = []Account{a}
	}
	// Judge each account's inbox
	type scored struct {
		msg Message
		p   float64
	}
	found := make([]scored, 0, 32)
	tags := map[string]string{}
	for _, a := range accs {
		msgs, _, err := m.list(ctx, a, FolderInbox)
		if err != nil {
			tags[a.Name] = err.Error()
			continue
		}
		msgs = newest(msgs, set.Limit)
		scores, err := m.svc.Spam(ctx, a, msgs)
		if err != nil {
			tags[a.Name] = "judge: " + err.Error()
			continue
		}
		for _, msg := range msgs {
			if p, ok := scores[msg.Key()]; ok && p >= spamFloor {
				found = append(found, scored{msg: msg, p: p})
			}
		}
	}
	// Most likely first
	sort.SliceStable(found, func(i, j int) bool { return found[i].p > found[j].p })
	// The lines, a mark column then the probability then the usual line
	m.mu.Lock()
	marks := m.marks
	m.mu.Unlock()
	marked := 0
	for _, s := range found {
		if marks[s.msg.Key()] {
			marked++
		}
	}
	lines := []string{fmt.Sprintf("%s spam  %d likely, %d marked", account, len(found), marked) + notes(tags), spamHint, ""}
	keys := []string{"", "", ""}
	msgs := make([]Message, 0, len(found))
	for _, s := range found {
		mark := " "
		if marks[s.msg.Key()] {
			mark = "*"
		}
		lines = append(lines, fmt.Sprintf("%s %.2f %s", mark, s.p, listLine(s.msg, account == AllAccounts)))
		keys = append(keys, s.msg.Key())
		msgs = append(msgs, s.msg)
	}
	// Remember so open and trash find the keys
	p := view.Page{Name: pageName(account, FolderSpam), Title: account + " spam", Lines: lines, Keys: keys, Cursor: 3, Filetype: "spam"}
	m.remember(p.Name, msgs)
	return p, nil
}

/**
 * mark
 * Toggles the mark on a message and shows the spam page again
 * @param ctx {context.Context} - the context
 * @param key {string} - the message key
 * @param account {string} - the account of the page
 * @return view.Response, error
 **/
func (m *Mail) mark(ctx context.Context, key, account string) (view.Response, error) {
	// Toggle
	if key == "" {
		return view.Response{Kind: view.KindNone}, nil
	}
	m.mu.Lock()
	next := make(map[string]bool, len(m.marks)+1)
	for k, v := range m.marks {
		next[k] = v
	}
	if next[key] {
		delete(next, key)
	} else {
		next[key] = true
	}
	m.marks = next
	m.mu.Unlock()
	return m.show(m.spamPage(ctx, account))
}

/**
 * purge
 * Trashes every marked message through the daemon and shows the spam page again
 * @param ctx {context.Context} - the context
 * @param account {string} - the account of the page
 * @return view.Response, error
 **/
func (m *Mail) purge(ctx context.Context, account string) (view.Response, error) {
	// Nothing to do without the service
	if m.svc.Trash == nil {
		return view.Fail("mail: trash is not available, set auth on the account"), nil
	}
	// The marked messages
	m.mu.Lock()
	marked := make([]string, 0, len(m.marks))
	for k := range m.marks {
		marked = append(marked, k)
	}
	m.mu.Unlock()
	if len(marked) == 0 {
		return view.Notify("mail: nothing marked"), nil
	}
	// Trash each one
	accs := m.src().Accounts
	done, failed := 0, 0
	for _, key := range marked {
		msg, ok := m.lookup(key)
		if !ok {
			continue
		}
		a, ok := findAccount(accs, msg.Account)
		if !ok {
			continue
		}
		if err := m.svc.Trash(ctx, a, msg); err != nil {
			failed++
			continue
		}
		done++
		m.mu.Lock()
		next := make(map[string]bool, len(m.marks))
		for k, v := range m.marks {
			if k != key {
				next[k] = v
			}
		}
		m.marks = next
		m.mu.Unlock()
	}
	// Show the page again
	resp, err := m.show(m.spamPage(ctx, account))
	if err == nil && resp.Kind == view.KindPage {
		resp.Text = fmt.Sprintf("mail: trashed %d, %d failed", done, failed)
	}
	return resp, err
}
