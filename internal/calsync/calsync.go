// Package calsync fetches calendars for the daemon and keeps the cache warm on each account's clock
package calsync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/choice404/symphony/internal/calendar"
	"github.com/choice404/symphony/internal/config"
	"github.com/choice404/symphony/internal/gcal"
	"github.com/choice404/symphony/internal/oauth"
)

// behind is how far back a warm cache reaches
const behind = 14

// ahead is how far past the configured days a warm cache reaches
const ahead = 14

/**
 * Fetch
 * Pulls every event of every calendar of an account in a window
 * @param ctx {context.Context} - the context
 * @param a {calendar.Account} - the account
 * @param from {time.Time} - the window start
 * @param to {time.Time} - the window end
 * @return []gcal.Event, error
 **/
func Fetch(ctx context.Context, a calendar.Account, from, to time.Time) ([]gcal.Event, error) {
	// The client
	c, err := client(ctx, a.Name)
	if err != nil {
		return nil, err
	}
	// Every calendar
	cals, err := c.Calendars(ctx)
	if err != nil {
		return nil, err
	}
	all := make([]gcal.Event, 0, 64)
	for _, cal := range cals {
		events, err := c.Events(ctx, cal.ID, from, to)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cal.Summary, err)
		}
		all = append(all, events...)
	}
	return all, nil
}

/**
 * Insert
 * Creates an event on the account's own calendar
 * @param ctx {context.Context} - the context
 * @param a {calendar.Account} - the account
 * @param e {gcal.Event} - the event
 * @return gcal.Event, error
 **/
func Insert(ctx context.Context, a calendar.Account, e gcal.Event) (gcal.Event, error) {
	c, err := client(ctx, a.Name)
	if err != nil {
		return gcal.Event{}, err
	}
	return c.Insert(ctx, gcal.Primary, e)
}

/**
 * Delete
 * Removes an event from a calendar
 * @param ctx {context.Context} - the context
 * @param a {calendar.Account} - the account
 * @param cal {string} - the calendar id
 * @param id {string} - the event id
 * @return error
 **/
func Delete(ctx context.Context, a calendar.Account, cal, id string) error {
	c, err := client(ctx, a.Name)
	if err != nil {
		return err
	}
	return c.Delete(ctx, cal, id)
}

/**
 * client
 * Builds a calendar client over the account's token
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @return *gcal.Client, error
 **/
func client(ctx context.Context, name string) (*gcal.Client, error) {
	h, err := oauth.HTTPClient(ctx, name)
	if err != nil {
		return nil, err
	}
	return gcal.New(h), nil
}

/**
 * Loop
 * Warms every oauth account's cache on its sync interval until the context ends
 * @param ctx {context.Context} - the context
 * @param load {func() config.Config} - returns the current config
 * @param store {*calendar.Store} - the cache
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return void
 **/
func Loop(ctx context.Context, load func() config.Config, store *calendar.Store, logf func(string, ...interface{})) {
	var wg sync.WaitGroup
	for _, a := range load().Mail.All() {
		if a.Auth != "oauth" || a.SyncEvery() == 0 {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			tick(ctx, name, load, store, logf)
		}(a.Name)
	}
	wg.Wait()
}

/**
 * tick
 * Warms one account's cache on its interval
 * @param ctx {context.Context} - the context
 * @param name {string} - the account name
 * @param load {func() config.Config} - returns the current config
 * @param store {*calendar.Store} - the cache
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return void
 **/
func tick(ctx context.Context, name string, load func() config.Config, store *calendar.Store, logf func(string, ...interface{})) {
	for {
		cfg := load()
		var acc config.Account
		found := false
		for _, a := range cfg.Mail.All() {
			if a.Name == name {
				acc, found = a, true
			}
		}
		if !found || acc.Auth != "oauth" {
			return
		}
		if err := Warm(ctx, acc, cfg.Calendar.Ahead(), store, logf); err != nil {
			logf("%s: calendar sync failed: %v", name, err)
		}
		every := acc.SyncEvery()
		if every == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

/**
 * Warm
 * Fetches an account's window into the cache once
 * @param ctx {context.Context} - the context
 * @param a {config.Account} - the account
 * @param days {int} - how far ahead the agenda looks
 * @param store {*calendar.Store} - the cache
 * @param logf {func(string, ...interface{})} - where log lines go
 * @return error
 **/
func Warm(ctx context.Context, a config.Account, days int, store *calendar.Store, logf func(string, ...interface{})) error {
	// The window, wide enough for a week either side of the agenda
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -behind)
	to := from.AddDate(0, 0, behind+days+ahead)
	events, err := Fetch(ctx, calendar.Account{Name: a.Name, User: a.User, Online: true}, from, to)
	if err != nil {
		return err
	}
	if err := store.Save(a.Name, calendar.Cached{FetchedAt: time.Now(), From: from, To: to, Events: events}); err != nil {
		return err
	}
	logf("%s: calendar synced, %d events", a.Name, len(events))
	return nil
}
