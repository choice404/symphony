package nvim

// NoColor is the value nvim sends when a highlight has no color set
const NoColor = -1

// Cell is one column of the grid, Text is a grapheme and an empty Text is the tail of a wide char
type Cell struct {
	// The grapheme drawn in this column
	Text string
	// The highlight id from hl_attr_define, 0 is the default
	Hl int
}

// Highlight is one decoded hl_attr_define entry
type Highlight struct {
	// The foreground as 24 bit rgb or NoColor
	Fg int
	// The background as 24 bit rgb or NoColor
	Bg int
	// The special color used for underlines or NoColor
	Sp int
	// Bold text
	Bold bool
	// Italic text
	Italic bool
	// Underlined text, any of the underline styles nvim sends
	Underline bool
	// Struck through text
	Strike bool
	// Swap the foreground and background
	Reverse bool
}

// CursorShape is how the cursor is drawn in the current mode
type CursorShape int

const (
	// CursorBlock fills the whole cell
	CursorBlock CursorShape = iota
	// CursorHorizontal is a line under the cell
	CursorHorizontal
	// CursorVertical is a bar at the left of the cell
	CursorVertical
)

// Screen is an immutable snapshot of everything needed to draw one frame
type Screen struct {
	// The grid width in columns
	Width int
	// The grid height in rows
	Height int
	// The cells, Height rows of Width cells
	Rows [][]Cell
	// The cursor row
	CursorRow int
	// The cursor column
	CursorCol int
	// The cursor shape for the current mode
	Cursor CursorShape
	// The current mode name such as normal or insert
	Mode string
	// The highlight table keyed by id
	Highlights map[int]Highlight
	// The default colors, Fg Bg and Sp only
	Default Highlight
	// The title nvim asked for
	Title string
}

/**
 * CellAt
 * Returns the cell at a row and column or an empty cell when out of range
 * @param row {int} - the row
 * @param col {int} - the column
 * @return Cell
 **/
func (s Screen) CellAt(row, col int) Cell {
	// Return the empty cell when the row is outside the grid
	if row < 0 || row >= len(s.Rows) {
		return Cell{}
	}
	// Return the empty cell when the column is outside the row
	if col < 0 || col >= len(s.Rows[row]) {
		return Cell{}
	}
	// Return the cell
	return s.Rows[row][col]
}

/**
 * Line
 * Returns the plain text of one row with highlights dropped
 * @param row {int} - the row
 * @return string
 **/
func (s Screen) Line(row int) string {
	// Return nothing when the row is outside the grid
	if row < 0 || row >= len(s.Rows) {
		return ""
	}
	// The text collected so far
	out := make([]byte, 0, s.Width)
	// Loop over every cell in the row
	for _, c := range s.Rows[row] {
		// Append the grapheme
		out = append(out, c.Text...)
	}
	// Return the joined text
	return string(out)
}

/**
 * HighlightFor
 * Returns the highlight for an id merged over the default colors
 * @param id {int} - the highlight id
 * @return Highlight
 **/
func (s Screen) HighlightFor(id int) Highlight {
	// Start from the default colors
	h := s.Default
	// Look up the id
	attr, ok := s.Highlights[id]
	// Return the defaults when the id is unknown or zero
	if !ok {
		return h
	}
	// Take the foreground when set
	if attr.Fg != NoColor {
		h.Fg = attr.Fg
	}
	// Take the background when set
	if attr.Bg != NoColor {
		h.Bg = attr.Bg
	}
	// Take the special color when set
	if attr.Sp != NoColor {
		h.Sp = attr.Sp
	}
	// Copy the flags
	h.Bold = attr.Bold
	h.Italic = attr.Italic
	h.Underline = attr.Underline
	h.Strike = attr.Strike
	h.Reverse = attr.Reverse
	// Return the merged highlight
	return h
}
