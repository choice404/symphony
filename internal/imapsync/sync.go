package imapsync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"
)

// batch is how many new messages one fetch asks for
const batch = 50

// Account is what one sync needs to know
type Account struct {
	// The account name, for log lines
	Name string
	// The Maildir root
	Dir string
	// The IMAP host with its port
	Host string
	// How many of the newest messages to keep
	Keep int
}

// Result counts what one sync did
type Result struct {
	// Messages written
	Added int
	// Local files removed because the server no longer has them in the window
	Removed int
	// Files renamed for a flag change
	Updated int
	// Messages in the window after the sync
	Total int
}

/**
 * Sync
 * Pulls the newest Keep messages of INBOX into the Maildir, drops local copies that fell out of the window, and renames files whose flags changed
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param auth {sasl.Client} - the login, OAUTHBEARER or PLAIN
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return Result, error
 **/
func Sync(ctx context.Context, a Account, auth sasl.Client, logf func(string, ...interface{})) (Result, error) {
	// The Maildir and its state
	var res Result
	if err := ensureMaildir(a.Dir); err != nil {
		return res, err
	}
	st, err := loadState(a.Dir)
	if err != nil {
		return res, err
	}
	// Connect and log in
	c, err := imapclient.DialTLS(a.Host, nil)
	if err != nil {
		return res, fmt.Errorf("dial %s: %w", a.Host, err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Authenticate(auth); err != nil {
		return res, fmt.Errorf("login %s: %w", a.Name, err)
	}
	defer func() { _ = c.Logout().Wait() }()
	// Open the inbox read only, this sync never changes the server
	sel, err := c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return res, fmt.Errorf("select: %w", err)
	}
	// A new UIDVALIDITY means every local file belongs to a mailbox that is gone
	if st.UIDValidity != 0 && st.UIDValidity != sel.UIDValidity {
		logf("%s: uidvalidity changed, starting over", a.Name)
		for uid, rel := range st.Files {
			_ = os.Remove(filepath.Join(a.Dir, rel))
			delete(st.Files, uid)
			res.Removed++
		}
	}
	st.UIDValidity = sel.UIDValidity
	// Every uid on the server, then the newest Keep of them
	want, err := window(c, a.Keep)
	if err != nil {
		return res, err
	}
	// Drop local files outside the window
	inWindow := map[uint32]bool{}
	for _, uid := range want {
		inWindow[uint32(uid)] = true
	}
	for uid, rel := range st.Files {
		if !inWindow[uid] {
			_ = os.Remove(filepath.Join(a.Dir, rel))
			delete(st.Files, uid)
			res.Removed++
		}
	}
	// Split the window into known and new
	known := make([]imap.UID, 0, len(want))
	fresh := make([]imap.UID, 0, len(want))
	for _, uid := range want {
		if _, ok := st.Files[uint32(uid)]; ok {
			known = append(known, uid)
		} else {
			fresh = append(fresh, uid)
		}
	}
	// Flags of the known ones
	if len(known) > 0 {
		n, err := updateFlags(c, a.Dir, &st, known)
		if err != nil {
			_ = saveState(a.Dir, st)
			return res, err
		}
		res.Updated = n
	}
	// The new ones, newest first so a cut off sync still leaves the top of the inbox
	sort.Slice(fresh, func(i, j int) bool { return fresh[i] > fresh[j] })
	host := hostName()
	for start := 0; start < len(fresh); start += batch {
		// Stop cleanly when asked
		if err := ctx.Err(); err != nil {
			_ = saveState(a.Dir, st)
			return res, err
		}
		end := start + batch
		if end > len(fresh) {
			end = len(fresh)
		}
		n, err := fetchNew(c, a.Dir, &st, fresh[start:end], host)
		res.Added += n
		if err != nil {
			_ = saveState(a.Dir, st)
			return res, err
		}
		if err := saveState(a.Dir, st); err != nil {
			return res, err
		}
	}
	// Done
	res.Total = len(st.Files)
	return res, saveState(a.Dir, st)
}

/**
 * window
 * Returns the newest keep uids of the selected mailbox in ascending order
 * @param c {*imapclient.Client} - the logged in client
 * @param keep {int} - how many
 * @return []imap.UID, error
 **/
func window(c *imapclient.Client, keep int) ([]imap.UID, error) {
	// Every uid
	data, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	set, ok := data.All.(imap.UIDSet)
	if !ok {
		return nil, nil
	}
	uids, ok := set.Nums()
	if !ok {
		return nil, fmt.Errorf("search: dynamic uid set")
	}
	// Ascending, then the tail
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	if keep > 0 && len(uids) > keep {
		uids = uids[len(uids)-keep:]
	}
	return uids, nil
}

/**
 * updateFlags
 * Fetches the flags of known uids and renames any file whose letters changed
 * @param c {*imapclient.Client} - the logged in client
 * @param dir {string} - the Maildir root
 * @param st {*State} - the state, updated in place
 * @param uids {[]imap.UID} - the known uids
 * @return int, error
 **/
func updateFlags(c *imapclient.Client, dir string, st *State, uids []imap.UID) (int, error) {
	// Fetch flags only
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return 0, fmt.Errorf("fetch flags: %w", err)
	}
	// Rename where the letters moved
	renamed := 0
	for _, m := range msgs {
		rel, ok := st.Files[uint32(m.UID)]
		if !ok {
			continue
		}
		next := withFlags(rel, flagLetters(m.Flags))
		if next == rel {
			continue
		}
		if err := os.Rename(filepath.Join(dir, rel), filepath.Join(dir, next)); err != nil {
			// A file someone deleted by hand is fetched again next time
			delete(st.Files, uint32(m.UID))
			continue
		}
		st.Files[uint32(m.UID)] = next
		renamed++
	}
	return renamed, nil
}

/**
 * fetchNew
 * Fetches whole messages for uids and writes each into the Maildir
 * @param c {*imapclient.Client} - the logged in client
 * @param dir {string} - the Maildir root
 * @param st {*State} - the state, updated in place
 * @param uids {[]imap.UID} - the uids to fetch
 * @param host {string} - this machine's name for file names
 * @return int, error
 **/
func fetchNew(c *imapclient.Client, dir string, st *State, uids []imap.UID, host string) (int, error) {
	// Fetch flags, the date, and the whole body without marking it seen
	opts := &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		InternalDate: true,
		BodySection:  []*imap.FetchItemBodySection{{Peek: true}},
	}
	msgs, err := c.Fetch(imap.UIDSetNum(uids...), opts).Collect()
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	// Write each one
	written := 0
	for _, m := range msgs {
		if len(m.BodySection) == 0 || len(m.BodySection[0].Bytes) == 0 {
			continue
		}
		when := m.InternalDate
		if when.IsZero() {
			when = time.Now()
		}
		rel, err := writeMessage(dir, uint32(m.UID), m.BodySection[0].Bytes, when, host, flagLetters(m.Flags))
		if err != nil {
			return written, err
		}
		st.Files[uint32(m.UID)] = rel
		written++
	}
	return written, nil
}

/**
 * OAuthBearer
 * Builds the OAUTHBEARER login Gmail takes for an access token
 * @param user {string} - the address
 * @param token {string} - the access token
 * @param host {string} - the IMAP host without its port
 * @return sasl.Client
 **/
func OAuthBearer(user, token, host string) sasl.Client {
	return sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{Username: user, Token: token, Host: host, Port: 993})
}

/**
 * Plain
 * Builds the PLAIN login for a password
 * @param user {string} - the address
 * @param password {string} - the password
 * @return sasl.Client
 **/
func Plain(user, password string) sasl.Client {
	return sasl.NewPlainClient("", user, password)
}
