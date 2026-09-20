package nvim

import "testing"

// ev builds one update the way the redraw handler receives it, a name then arg tuples
func ev(name string, tuples ...[]interface{}) []interface{} {
	// The update starting with the name
	u := []interface{}{name}
	// Append every tuple
	for _, t := range tuples {
		u = append(u, t)
	}
	// Return the update
	return u
}

// args builds one arg tuple with ints as int64 the way msgpack decodes them
func args(v ...interface{}) []interface{} {
	// The converted tuple
	out := make([]interface{}, len(v))
	// Loop over every value
	for i, x := range v {
		// Widen plain ints to int64
		if n, ok := x.(int); ok {
			out[i] = int64(n)
			continue
		}
		// Keep everything else
		out[i] = x
	}
	// Return the tuple
	return out
}

// cell builds one grid_line cell tuple
func cell(v ...interface{}) []interface{} {
	return args(v...)
}

// mustApply applies an update and fails the test on error
func mustApply(t *testing.T, s *state, u []interface{}) {
	t.Helper()
	// Apply and fail on any error
	if err := s.apply(u); err != nil {
		t.Fatalf("apply %v: %v", u[0], err)
	}
}

// sized builds a state resized to a width and height
func sized(t *testing.T, w, h int) *state {
	t.Helper()
	// The fresh state
	s := newState()
	// Resize it through the event path
	mustApply(t, s, ev("grid_resize", args(1, w, h)))
	// Return it
	return s
}

func TestGridResizeBlanksAndKeeps(t *testing.T) {
	s := sized(t, 4, 2)
	// Every cell starts as a space
	if got := s.snapshot().Line(0); got != "    " {
		t.Fatalf("row 0 = %q", got)
	}
	// Write a row then shrink and grow
	mustApply(t, s, ev("grid_line", args(1, 0, 0, []interface{}{cell("a"), cell("b"), cell("c"), cell("d")}, false)))
	mustApply(t, s, ev("grid_resize", args(1, 2, 1)))
	if got := s.snapshot().Line(0); got != "ab" {
		t.Fatalf("after shrink row 0 = %q", got)
	}
	mustApply(t, s, ev("grid_resize", args(1, 3, 2)))
	if got := s.snapshot().Line(0); got != "ab " {
		t.Fatalf("after grow row 0 = %q", got)
	}
	if got := s.snapshot().Line(1); got != "   " {
		t.Fatalf("after grow row 1 = %q", got)
	}
}

func TestGridLineRepeatAndHlCarry(t *testing.T) {
	s := sized(t, 6, 1)
	// A cell with hl 5 then a repeat cell with no hl then a cell with hl 0
	cells := []interface{}{cell("x", 5), cell("-", nil, 3), cell("y", 0)}
	mustApply(t, s, ev("grid_line", args(1, 0, 1, cells, false)))
	snap := s.snapshot()
	if got := snap.Line(0); got != " x---y" {
		t.Fatalf("row = %q", got)
	}
	// The repeat cells carry hl 5 from the cell before them
	for col := 1; col <= 4; col++ {
		if snap.CellAt(0, col).Hl != 5 {
			t.Fatalf("col %d hl = %d want 5", col, snap.CellAt(0, col).Hl)
		}
	}
	if snap.CellAt(0, 5).Hl != 0 {
		t.Fatalf("col 5 hl = %d want 0", snap.CellAt(0, 5).Hl)
	}
}

func TestGridLineBatchedTuples(t *testing.T) {
	s := sized(t, 2, 2)
	// One update carrying two rows
	mustApply(t, s, ev("grid_line",
		args(1, 0, 0, []interface{}{cell("a", 1), cell("b")}, false),
		args(1, 1, 0, []interface{}{cell("c", 1), cell("d")}, false),
	))
	snap := s.snapshot()
	if snap.Line(0) != "ab" || snap.Line(1) != "cd" {
		t.Fatalf("rows = %q %q", snap.Line(0), snap.Line(1))
	}
}

func TestGridLineClipsAtWidth(t *testing.T) {
	s := sized(t, 3, 1)
	// A repeat past the right edge must not panic
	mustApply(t, s, ev("grid_line", args(1, 0, 1, []interface{}{cell("z", 1, 10)}, false)))
	if got := s.snapshot().Line(0); got != " zz" {
		t.Fatalf("row = %q", got)
	}
}

func TestGridLineRejectsBadRow(t *testing.T) {
	s := sized(t, 3, 1)
	// A row outside the grid is an error not a panic
	if err := s.apply(ev("grid_line", args(1, 7, 0, []interface{}{cell("z", 1)}, false))); err == nil {
		t.Fatal("expected error for row outside grid")
	}
}

func TestGridClear(t *testing.T) {
	s := sized(t, 2, 1)
	mustApply(t, s, ev("grid_line", args(1, 0, 0, []interface{}{cell("a", 1), cell("b")}, false)))
	mustApply(t, s, ev("grid_clear", args(1)))
	if got := s.snapshot().Line(0); got != "  " {
		t.Fatalf("row = %q", got)
	}
}

// fillRows writes one letter per row so scroll direction is visible
func fillRows(t *testing.T, s *state, letters string) {
	t.Helper()
	for r, ch := range letters {
		mustApply(t, s, ev("grid_line", args(1, r, 0, []interface{}{cell(string(ch), 1)}, false)))
	}
}

func TestGridScrollUp(t *testing.T) {
	s := sized(t, 1, 4)
	fillRows(t, s, "abcd")
	// Scroll the whole grid up by one, rows 0..2 take rows 1..3 and row 3 is left as is
	mustApply(t, s, ev("grid_scroll", args(1, 0, 4, 0, 1, 1, 0)))
	snap := s.snapshot()
	got := snap.Line(0) + snap.Line(1) + snap.Line(2) + snap.Line(3)
	if got != "bcdd" {
		t.Fatalf("rows = %q want bcdd", got)
	}
}

func TestGridScrollDown(t *testing.T) {
	s := sized(t, 1, 4)
	fillRows(t, s, "abcd")
	// Scroll the whole grid down by one, rows 1..3 take rows 0..2 and row 0 is left as is
	mustApply(t, s, ev("grid_scroll", args(1, 0, 4, 0, 1, -1, 0)))
	snap := s.snapshot()
	got := snap.Line(0) + snap.Line(1) + snap.Line(2) + snap.Line(3)
	if got != "aabc" {
		t.Fatalf("rows = %q want aabc", got)
	}
}

func TestGridScrollRegionOnly(t *testing.T) {
	s := sized(t, 2, 3)
	// Two columns, only the left one scrolls
	for r, ch := range "abc" {
		mustApply(t, s, ev("grid_line", args(1, r, 0, []interface{}{cell(string(ch), 1), cell("x")}, false)))
	}
	mustApply(t, s, ev("grid_scroll", args(1, 0, 3, 0, 1, 1, 0)))
	snap := s.snapshot()
	if snap.Line(0) != "bx" || snap.Line(1) != "cx" || snap.Line(2) != "cx" {
		t.Fatalf("rows = %q %q %q", snap.Line(0), snap.Line(1), snap.Line(2))
	}
}

func TestCursorGotoClamps(t *testing.T) {
	s := sized(t, 3, 2)
	mustApply(t, s, ev("grid_cursor_goto", args(1, 1, 2)))
	snap := s.snapshot()
	if snap.CursorRow != 1 || snap.CursorCol != 2 {
		t.Fatalf("cursor = %d,%d", snap.CursorRow, snap.CursorCol)
	}
	mustApply(t, s, ev("grid_cursor_goto", args(1, 9, 9)))
	snap = s.snapshot()
	if snap.CursorRow != 1 || snap.CursorCol != 2 {
		t.Fatalf("clamped cursor = %d,%d", snap.CursorRow, snap.CursorCol)
	}
}

func TestHighlightsAndDefaults(t *testing.T) {
	s := sized(t, 1, 1)
	mustApply(t, s, ev("default_colors_set", args(0xffffff, 0x000000, 0xff0000, 0, 0)))
	attr := map[string]interface{}{"foreground": int64(0x00ff00), "bold": true, "undercurl": true}
	mustApply(t, s, ev("hl_attr_define", args(7, attr, map[string]interface{}{}, []interface{}{})))
	snap := s.snapshot()
	h := snap.HighlightFor(7)
	if h.Fg != 0x00ff00 || h.Bg != 0x000000 || !h.Bold || !h.Underline {
		t.Fatalf("merged highlight = %+v", h)
	}
	// An unknown id is the defaults
	d := snap.HighlightFor(99)
	if d.Fg != 0xffffff || d.Bg != 0x000000 || d.Bold {
		t.Fatalf("default highlight = %+v", d)
	}
}

func TestModeInfoAndChange(t *testing.T) {
	s := sized(t, 1, 1)
	modes := []interface{}{
		map[string]interface{}{"name": "normal", "cursor_shape": "block"},
		map[string]interface{}{"name": "insert", "cursor_shape": "vertical"},
	}
	mustApply(t, s, ev("mode_info_set", args(true, modes)))
	mustApply(t, s, ev("mode_change", args("insert", 1)))
	snap := s.snapshot()
	if snap.Mode != "insert" || snap.Cursor != CursorVertical {
		t.Fatalf("mode = %q cursor = %v", snap.Mode, snap.Cursor)
	}
}

func TestSnapshotIsIndependent(t *testing.T) {
	s := sized(t, 2, 1)
	mustApply(t, s, ev("grid_line", args(1, 0, 0, []interface{}{cell("a", 1), cell("b")}, false)))
	snap := s.snapshot()
	// Changing the state after the snapshot must not change the snapshot
	mustApply(t, s, ev("grid_clear", args(1)))
	mustApply(t, s, ev("hl_attr_define", args(1, map[string]interface{}{"bold": true}, map[string]interface{}{}, []interface{}{})))
	if snap.Line(0) != "ab" {
		t.Fatalf("snapshot row changed to %q", snap.Line(0))
	}
	if snap.Highlights[1].Bold {
		t.Fatal("snapshot highlight table changed")
	}
}

func TestUnknownEventIgnored(t *testing.T) {
	s := sized(t, 1, 1)
	if err := s.apply(ev("win_viewport", args(2, 1000, 0, 5, 0, 0, 5, 0))); err != nil {
		t.Fatalf("unknown event errored: %v", err)
	}
}

func TestOtherGridIgnored(t *testing.T) {
	s := sized(t, 2, 1)
	mustApply(t, s, ev("grid_line", args(2, 0, 0, []interface{}{cell("z", 1), cell("z")}, false)))
	if got := s.snapshot().Line(0); got != "  " {
		t.Fatalf("grid 2 leaked into grid 1: %q", got)
	}
}
