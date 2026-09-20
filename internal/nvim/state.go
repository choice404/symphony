package nvim

// state is the mutable grid owned by the redraw goroutine alone, it never leaves that goroutine, only Screen snapshots do
type state struct {
	// The grid width in columns
	width int
	// The grid height in rows
	height int
	// The cells, height rows of width cells
	rows [][]Cell
	// The cursor row
	cursorRow int
	// The cursor column
	cursorCol int
	// The index into modes for the current mode
	modeIdx int
	// The cursor shape of every mode from mode_info_set
	modeShapes []CursorShape
	// The name of every mode from mode_info_set
	modeNames []string
	// The current mode name from mode_change
	mode string
	// The highlight table keyed by id
	highlights map[int]Highlight
	// The default colors
	def Highlight
	// The title nvim asked for
	title string
}

/**
 * newState
 * Builds an empty state with no colors set
 * @return *state
 **/
func newState() *state {
	// Return a state with an empty highlight table and no default colors
	return &state{
		highlights: map[int]Highlight{},
		def:        Highlight{Fg: NoColor, Bg: NoColor, Sp: NoColor},
	}
}

/**
 * blankRow
 * Builds one row of blank cells
 * @param width {int} - the row width
 * @return []Cell
 **/
func blankRow(width int) []Cell {
	// The new row
	row := make([]Cell, width)
	// Loop over every cell
	for i := range row {
		// Fill it with a space in the default highlight
		row[i] = Cell{Text: " "}
	}
	// Return the row
	return row
}

/**
 * resize
 * Resizes the grid keeping whatever content still fits
 * @param width {int} - the new width
 * @param height {int} - the new height
 * @return void
 **/
func (s *state) resize(width, height int) {
	// The new rows
	rows := make([][]Cell, height)
	// Loop over every new row
	for r := range rows {
		// Start from a blank row
		rows[r] = blankRow(width)
		// Skip the copy when the old grid had no such row
		if r >= len(s.rows) {
			continue
		}
		// Copy as many old cells as fit
		copy(rows[r], s.rows[r])
	}
	// Store the new size and rows
	s.width = width
	s.height = height
	s.rows = rows
	// Clamp the cursor into the new grid
	s.cursorRow = clamp(s.cursorRow, 0, height-1)
	s.cursorCol = clamp(s.cursorCol, 0, width-1)
}

/**
 * clear
 * Blanks every cell of the grid
 * @return void
 **/
func (s *state) clear() {
	// Loop over every row
	for r := range s.rows {
		// Replace it with a blank row
		s.rows[r] = blankRow(s.width)
	}
}

/**
 * cursorShape
 * Returns the cursor shape for the current mode
 * @return CursorShape
 **/
func (s *state) cursorShape() CursorShape {
	// Fall back to a block when the mode index is unknown
	if s.modeIdx < 0 || s.modeIdx >= len(s.modeShapes) {
		return CursorBlock
	}
	// Return the shape for the mode
	return s.modeShapes[s.modeIdx]
}

/**
 * snapshot
 * Copies the state into an immutable Screen safe to hand to another goroutine
 * @return Screen
 **/
func (s *state) snapshot() Screen {
	// The copied rows
	rows := make([][]Cell, len(s.rows))
	// Loop over every row
	for r := range s.rows {
		// Copy the cells
		rows[r] = make([]Cell, len(s.rows[r]))
		copy(rows[r], s.rows[r])
	}
	// The copied highlight table
	hl := make(map[int]Highlight, len(s.highlights))
	// Loop over every highlight
	for id, h := range s.highlights {
		// Copy the entry
		hl[id] = h
	}
	// Return the snapshot
	return Screen{
		Width:      s.width,
		Height:     s.height,
		Rows:       rows,
		CursorRow:  s.cursorRow,
		CursorCol:  s.cursorCol,
		Cursor:     s.cursorShape(),
		Mode:       s.mode,
		Highlights: hl,
		Default:    s.def,
		Title:      s.title,
	}
}

/**
 * clamp
 * Clamps a value into a range
 * @param v {int} - the value
 * @param lo {int} - the low bound
 * @param hi {int} - the high bound
 * @return int
 **/
func clamp(v, lo, hi int) int {
	// Return the low bound when the range is empty
	if hi < lo {
		return lo
	}
	// Clamp below
	if v < lo {
		return lo
	}
	// Clamp above
	if v > hi {
		return hi
	}
	// Return the value
	return v
}
