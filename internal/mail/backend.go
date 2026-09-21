package mail

import (
	"context"
)

// Backend is where messages come from, the plain scanner or the contract, the view is the same over both
type Backend interface {
	// List reads a folder of an account newest first, with a health tag such as partial for the header, an error when the account is unusable
	List(ctx context.Context, a Account, folder string) ([]Message, string, error)
	// Open reads one message in full
	Open(ctx context.Context, a Account, m Message) (Opened, error)
	// Label gives each message its label, a copy is returned and the input is left alone
	Label(ctx context.Context, a Account, msgs []Message) []Message
	// Refresh drops whatever the backend cached so the next List starts fresh
	Refresh()
}

// Plain reads Maildirs straight from disk
type Plain struct{}

/**
 * List
 * Scans a folder's Maildir
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param folder {string} - inbox or sent
 * @return []Message, string, error
 **/
func (Plain) List(ctx context.Context, a Account, folder string) ([]Message, string, error) {
	// Scan the folder's directory
	msgs, err := Scan(FolderDir(a.Dir, folder))
	if err != nil {
		return nil, "", err
	}
	// Tag every message
	for i := range msgs {
		msgs[i].Account = a.Name
		msgs[i].Folder = folder
	}
	return msgs, "", nil
}

/**
 * Open
 * Reads one message file
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param m {Message} - the message
 * @return Opened, error
 **/
func (Plain) Open(ctx context.Context, a Account, m Message) (Opened, error) {
	// Read the file
	o, err := Open(m.Path)
	if err != nil {
		return Opened{}, err
	}
	o.Account = m.Account
	o.Folder = m.Folder
	return o, nil
}

/**
 * Label
 * Returns the messages unchanged, the plain backend has no classifier
 * @param ctx {context.Context} - the context
 * @param a {Account} - the account
 * @param msgs {[]Message} - the messages
 * @return []Message
 **/
func (Plain) Label(ctx context.Context, a Account, msgs []Message) []Message {
	return msgs
}

/**
 * Refresh
 * Nothing is cached
 * @return void
 **/
func (Plain) Refresh() {}
