package nvim

import (
	"regexp"
	"strings"
	"testing"
)

// ansi matches every escape sequence the renderer emits
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// strip removes every escape sequence
func strip(s string) string {
	return ansi.ReplaceAllString(s, "")
}

// screen builds a snapshot from a few events
func screen(t *testing.T, w, h int, updates ...[]interface{}) Screen {
	t.Helper()
	s := sized(t, w, h)
	for _, u := range updates {
		mustApply(t, s, u)
	}
	return s.snapshot()
}

func TestRenderPlainText(t *testing.T) {
	snap := screen(t, 3, 2,
		ev("grid_line", args(1, 0, 0, []interface{}{cell("a", 0), cell("b"), cell("c")}, false)),
		ev("grid_line", args(1, 1, 0, []interface{}{cell("d", 0), cell(" ", 0, 2)}, false)),
		ev("grid_cursor_goto", args(1, 1, 2)),
	)
	out := Render(snap)
	// Two lines, no trailing newline, text intact once escapes are stripped
	if got := strip(out); got != "abc\nd  " {
		t.Fatalf("stripped = %q", got)
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatal("trailing newline")
	}
}

func TestRenderColorRun(t *testing.T) {
	attr := map[string]interface{}{"foreground": int64(0xff0000), "background": int64(0x0000ff), "bold": true}
	snap := screen(t, 3, 1,
		ev("hl_attr_define", args(3, attr, map[string]interface{}{}, []interface{}{})),
		ev("grid_line", args(1, 0, 0, []interface{}{cell("x", 3), cell("y"), cell("z", 0)}, false)),
		ev("grid_cursor_goto", args(1, 0, 2)),
	)
	out := Render(snap)
	// The colored run carries bold, a red foreground, and a blue background before xy
	want := "\x1b[1;38;2;255;0;0;48;2;0;0;255mxy"
	if !strings.Contains(out, want) {
		t.Fatalf("missing colored run in %q", out)
	}
	// The cursor cell in normal mode is reversed
	if !strings.Contains(out, "\x1b[7mz") {
		t.Fatalf("missing reversed cursor cell in %q", out)
	}
}

func TestRenderCursorUnderlineInInsert(t *testing.T) {
	modes := []interface{}{
		map[string]interface{}{"name": "normal", "cursor_shape": "block"},
		map[string]interface{}{"name": "insert", "cursor_shape": "vertical"},
	}
	snap := screen(t, 1, 1,
		ev("mode_info_set", args(true, modes)),
		ev("mode_change", args("insert", 1)),
		ev("grid_line", args(1, 0, 0, []interface{}{cell("q", 0)}, false)),
	)
	out := Render(snap)
	if !strings.Contains(out, "\x1b[4mq") {
		t.Fatalf("missing underlined cursor cell in %q", out)
	}
}

func TestRenderSkipsWideTail(t *testing.T) {
	snap := screen(t, 3, 1,
		ev("grid_line", args(1, 0, 0, []interface{}{cell("漢", 0), cell(""), cell("a")}, false)),
		ev("grid_cursor_goto", args(1, 0, 2)),
	)
	if got := strip(Render(snap)); got != "漢a" {
		t.Fatalf("stripped = %q", got)
	}
}

func TestSgrReverseSwapsColors(t *testing.T) {
	h := Highlight{Fg: 0x111111, Bg: 0x222222, Sp: NoColor, Reverse: true}
	got := sgr(h, false, CursorBlock)
	if got != "\x1b[38;2;34;34;34;48;2;17;17;17m" {
		t.Fatalf("sgr = %q", got)
	}
}

func TestSgrEmpty(t *testing.T) {
	h := Highlight{Fg: NoColor, Bg: NoColor, Sp: NoColor}
	if got := sgr(h, false, CursorBlock); got != "" {
		t.Fatalf("sgr = %q", got)
	}
}
