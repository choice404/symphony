package gitapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/choice404/symphony/internal/view"
)

// repo makes a repository with one commit, a staged change, an unstaged change, and an untracked file
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@x", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	g("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", "a.txt")
	g("commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g("add", "a.txt")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestParseStatus(t *testing.T) {
	out := "# branch.oid abc\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -1\n1 MM N... 100644 100644 100644 h h a.txt\n1 .D N... 100644 100644 000000 h h gone.txt\n2 R. N... 100644 100644 100644 h h R100 new.txt\told.txt\n? untracked.txt\n"
	st := parseStatus(out)
	if st.Branch != "main" || st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("branch = %+v", st)
	}
	if len(st.StagedList) != 2 || st.StagedList[0].Path != "a.txt" || st.StagedList[1].Path != "new.txt" || st.StagedList[1].Staged != 'R' {
		t.Fatalf("staged = %+v", st.StagedList)
	}
	if len(st.UnstagedList) != 2 || st.UnstagedList[1].Path != "gone.txt" || st.UnstagedList[1].Unstaged != 'D' {
		t.Fatalf("unstaged = %+v", st.UnstagedList)
	}
	if len(st.UntrackedList) != 1 || st.UntrackedList[0].Path != "untracked.txt" {
		t.Fatalf("untracked = %+v", st.UntrackedList)
	}
	if parseStatus("# branch.head (detached)\n").Branch != "" {
		t.Fatal("detached should have no branch")
	}
}

func TestStatusPageAndDiff(t *testing.T) {
	dir := repo(t)
	g := New()
	ctx := context.Background()
	p, err := g.RenderPath(ctx, "status/"+dir)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(p.Lines, "\n")
	if !strings.HasPrefix(p.Lines[0], "main  ") || !strings.Contains(text, "staged\n  M  a.txt") || !strings.Contains(text, "unstaged\n  M  a.txt") || !strings.Contains(text, "untracked\n  ?  new.txt") {
		t.Fatalf("status = %s", text)
	}
	if p.Keys[4] != "staged:a.txt" || p.Filetype != "gitstatus" || p.Cursor != 4 {
		t.Fatalf("keys = %v cursor = %d", p.Keys, p.Cursor)
	}
	// The diff page carries both parts
	r, _ := g.Act(ctx, view.Action{Name: "open", Key: "staged:a.txt", Page: "git/status/" + dir})
	if r.Kind != view.KindPage || r.Page.Filetype != "diff" {
		t.Fatalf("diff = %+v", r)
	}
	text = strings.Join(r.Page.Lines, "\n")
	if !strings.Contains(text, "# staged") || !strings.Contains(text, "+two") || !strings.Contains(text, "# unstaged") || !strings.Contains(text, "+three") {
		t.Fatalf("diff page = %s", text)
	}
	// An untracked file shows as a whole addition
	r, _ = g.Act(ctx, view.Action{Name: "open", Key: "untracked:new.txt", Page: "git/status/" + dir})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "+new") {
		t.Fatalf("untracked diff = %v", r.Page.Lines)
	}
}

func TestStageUnstageCommitLog(t *testing.T) {
	dir := repo(t)
	g := New()
	ctx := context.Background()
	page := "git/status/" + dir
	// Stage the untracked file and the unstaged change
	r, _ := g.Act(ctx, view.Action{Name: "stage", Key: "untracked:new.txt", Page: page})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "  A  new.txt") {
		t.Fatalf("after stage = %v", r.Page.Lines)
	}
	r, _ = g.Act(ctx, view.Action{Name: "unstage", Key: "staged:new.txt", Page: page})
	if !strings.Contains(strings.Join(r.Page.Lines, "\n"), "  ?  new.txt") {
		t.Fatalf("after unstage = %v", r.Page.Lines)
	}
	// A path outside the repository is refused
	if err := Stage(ctx, dir, "../x"); err == nil {
		t.Fatal("escaped path staged")
	}
	// The commit page shows the staged stat, and committing lands in the log
	r, _ = g.Act(ctx, view.Action{Name: "commit", Page: page})
	if r.Kind != view.KindPage || !r.Page.Editable || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "a.txt") {
		t.Fatalf("commit page = %+v", r)
	}
	lines := r.Page.Lines
	lines[1] = "second line added"
	r, _ = g.Act(ctx, view.Action{Name: "save", Key: r.Page.Key, Page: r.Page.Name, Body: strings.Join(lines, "\n")})
	if r.Kind != view.KindPage || r.Close != "git/commit/"+dir+"//1" || !strings.Contains(r.Text, "second line added") {
		t.Fatalf("commit = %+v", r)
	}
	if strings.Contains(strings.Join(r.Page.Lines, "\n"), "\nstaged\n") {
		t.Fatal("staged section still present after commit")
	}
	lp, err := g.RenderPath(ctx, "log/"+dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lp.Lines) != 5 || !strings.Contains(lp.Lines[3], "second line added") || !strings.Contains(lp.Lines[4], "first") {
		t.Fatalf("log = %v", lp.Lines)
	}
	// Showing a commit gives its patch
	r, _ = g.Act(ctx, view.Action{Name: "open", Key: lp.Keys[3], Page: "git/log/" + dir})
	if r.Kind != view.KindPage || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "+two") {
		t.Fatalf("show = %+v", r)
	}
	// An empty message is refused
	r, _ = g.Act(ctx, view.Action{Name: "commit", Page: page})
	if r.Kind != view.KindNotify {
		t.Fatalf("commit with nothing staged = %+v", r)
	}
}

func TestPushWithoutRemoteReportsOnPage(t *testing.T) {
	dir := repo(t)
	r, _ := New().Act(context.Background(), view.Action{Name: "push", Page: "git/status/" + dir})
	if r.Kind != view.KindPage || !strings.Contains(strings.Join(r.Page.Lines, "\n"), "git push") {
		t.Fatalf("push = %+v", r)
	}
}
