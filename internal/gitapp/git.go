// Package gitapp shows a repository's status, diffs, and log and runs the everyday git commands through git itself
package gitapp

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// timeout is how long one git command may take, pushes and pulls get longer
const timeout = 10 * time.Second

// netTimeout is how long a push or pull may take
const netTimeout = 2 * time.Minute

// logCount is how many commits the log page lists
const logCount = 200

// Entry is one path in the status
type Entry struct {
	// The path relative to the repository
	Path string
	// The staged status letter, M A D R or space
	Staged byte
	// The unstaged status letter, M D or space
	Unstaged byte
	// Whether git does not track it
	Untracked bool
}

// Status is the repository state
type Status struct {
	// The branch, empty on a detached head
	Branch string
	// Commits ahead of the upstream
	Ahead int
	// Commits behind the upstream
	Behind int
	// The entries with something staged
	StagedList []Entry
	// The entries with something unstaged
	UnstagedList []Entry
	// The untracked entries
	UntrackedList []Entry
}

// LogEntry is one line of the log
type LogEntry struct {
	// The short hash
	Hash string
	// The author name
	Author string
	// How long ago, as git says it
	When string
	// The subject
	Subject string
}

/**
 * run
 * Runs git in a repository with a timeout and returns its output, the error carries stderr
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param stdin {string} - what to feed it, empty for nothing
 * @param limit {time.Duration} - the timeout
 * @param args {...string} - the git arguments
 * @return string, error
 **/
func run(ctx context.Context, repo, stdin string, limit time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out.String(), nil
}

/**
 * ReadStatus
 * Reads the branch, the upstream distance, and every changed path
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return Status, error
 **/
func ReadStatus(ctx context.Context, repo string) (Status, error) {
	out, err := run(ctx, repo, "", timeout, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out), nil
}

/**
 * parseStatus
 * Parses porcelain v2 output
 * @param out {string} - the output
 * @return Status
 **/
func parseStatus(out string) Status {
	var st Status
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head != "(detached)" {
				st.Branch = head
			}
		case strings.HasPrefix(line, "# branch.ab "):
			// +N -M
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) == 2 {
				st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
				st.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			}
		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 "):
			// 1 XY sub mH mI mW hH hI path, or 2 with a score and orig path after a tab
			fields := strings.SplitN(line, " ", 9)
			if len(fields) < 9 {
				continue
			}
			path := fields[8]
			if strings.HasPrefix(line, "2 ") {
				fields = strings.SplitN(line, " ", 10)
				if len(fields) < 10 {
					continue
				}
				path = strings.SplitN(fields[9], "\t", 2)[0]
			}
			xy := fields[1]
			e := Entry{Path: path, Staged: xy[0], Unstaged: xy[1]}
			if e.Staged != '.' {
				st.StagedList = append(st.StagedList, e)
			}
			if e.Unstaged != '.' {
				st.UnstagedList = append(st.UnstagedList, e)
			}
		case strings.HasPrefix(line, "? "):
			st.UntrackedList = append(st.UntrackedList, Entry{Path: strings.TrimPrefix(line, "? "), Untracked: true})
		}
	}
	return st
}

/**
 * safe
 * Checks that a path from a page key stays inside the repository
 * @param repo {string} - the repository
 * @param rel {string} - the relative path
 * @return error
 **/
func safe(repo, rel string) error {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || strings.Contains(rel, "/../") {
		return fmt.Errorf("git: %q is not a path in the repository", rel)
	}
	return nil
}

/**
 * Stage
 * Stages a path
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param rel {string} - the path
 * @return error
 **/
func Stage(ctx context.Context, repo, rel string) error {
	if err := safe(repo, rel); err != nil {
		return err
	}
	_, err := run(ctx, repo, "", timeout, "add", "--", rel)
	return err
}

/**
 * Unstage
 * Takes a path out of the index
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param rel {string} - the path
 * @return error
 **/
func Unstage(ctx context.Context, repo, rel string) error {
	if err := safe(repo, rel); err != nil {
		return err
	}
	_, err := run(ctx, repo, "", timeout, "restore", "--staged", "--", rel)
	return err
}

/**
 * Diff
 * Returns the staged and unstaged diffs of a path, an untracked file as a whole addition
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param rel {string} - the path
 * @param untracked {bool} - whether git does not track it
 * @return string, string, error
 **/
func Diff(ctx context.Context, repo, rel string, untracked bool) (string, string, error) {
	if err := safe(repo, rel); err != nil {
		return "", "", err
	}
	if untracked {
		// No index diff exits 1 when there are differences, which is the point
		out, _ := run(ctx, repo, "", timeout, "diff", "--no-index", "--", "/dev/null", rel)
		return "", out, nil
	}
	staged, err := run(ctx, repo, "", timeout, "diff", "--cached", "--", rel)
	if err != nil {
		return "", "", err
	}
	unstaged, err := run(ctx, repo, "", timeout, "diff", "--", rel)
	if err != nil {
		return "", "", err
	}
	return staged, unstaged, nil
}

/**
 * StagedStat
 * Returns the stat of what is staged, for the commit page
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return string, error
 **/
func StagedStat(ctx context.Context, repo string) (string, error) {
	return run(ctx, repo, "", timeout, "diff", "--cached", "--stat")
}

/**
 * Commit
 * Commits the index with a message
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param message {string} - the message
 * @return string, error
 **/
func Commit(ctx context.Context, repo, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("git: the message is empty")
	}
	out, err := run(ctx, repo, message, timeout, "commit", "-F", "-")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

/**
 * Log
 * Returns the newest commits
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return []LogEntry, error
 **/
func Log(ctx context.Context, repo string) ([]LogEntry, error) {
	out, err := run(ctx, repo, "", timeout, "log", "-n", strconv.Itoa(logCount), "--format=%h%x00%an%x00%ar%x00%s")
	if err != nil {
		// A repository with no commits yet has nothing to log
		if strings.Contains(err.Error(), "does not have any commits") {
			return nil, nil
		}
		return nil, err
	}
	commits := make([]LogEntry, 0, logCount)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, LogEntry{Hash: parts[0], Author: parts[1], When: parts[2], Subject: parts[3]})
	}
	return commits, nil
}

/**
 * Show
 * Returns one commit with its stat and patch
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @param hash {string} - the commit
 * @return string, error
 **/
func Show(ctx context.Context, repo, hash string) (string, error) {
	if hash == "" || strings.HasPrefix(hash, "-") {
		return "", fmt.Errorf("git: bad commit %q", hash)
	}
	return run(ctx, repo, "", timeout, "show", "--stat", "--patch", "--format=commit %H%nAuthor: %an <%ae>%nDate:   %ad%n%n    %s%n%n%b", hash)
}

/**
 * Push
 * Pushes the current branch, output and error text both returned so a page can show them
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return string, error
 **/
func Push(ctx context.Context, repo string) (string, error) {
	return run(ctx, repo, "", netTimeout, "push")
}

/**
 * Pull
 * Fast forwards the current branch from its upstream
 * @param ctx {context.Context} - the context
 * @param repo {string} - the repository
 * @return string, error
 **/
func Pull(ctx context.Context, repo string) (string, error) {
	return run(ctx, repo, "", netTimeout, "pull", "--ff-only")
}
