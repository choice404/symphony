package nvim

import (
	"strconv"
	"strings"
)

// reset clears every attribute
const reset = "\x1b[0m"

/**
 * Render
 * Turns a Screen into the ANSI text bubbletea draws, one line per row with no trailing newline
 * @param s {Screen} - the snapshot to draw
 * @return string
 **/
func Render(s Screen) string {
	// The lines collected so far
	lines := make([]string, 0, s.Height)
	// Loop over every row
	for r := 0; r < s.Height; r++ {
		// Render the row
		lines = append(lines, renderRow(s, r))
	}
	// Join the rows with newlines
	return strings.Join(lines, "\n")
}

/**
 * renderRow
 * Renders one row as runs of cells that share a highlight
 * @param s {Screen} - the snapshot
 * @param r {int} - the row
 * @return string
 **/
func renderRow(s Screen, r int) string {
	// The output for this row
	var b strings.Builder
	// The highlight id of the open run, -1 when none is open
	openHl := -1
	// Whether the open run is the cursor cell
	openCursor := false
	// Loop over every column
	for c := 0; c < s.Width; c++ {
		// Read the cell
		cell := s.CellAt(r, c)
		// Skip the tail of a wide char, the head already covers it
		if cell.Text == "" {
			continue
		}
		// Whether this cell holds the cursor
		cursor := r == s.CursorRow && c == s.CursorCol
		// Open a new run when the highlight or the cursor state changed
		if cell.Hl != openHl || cursor != openCursor {
			b.WriteString(reset)
			b.WriteString(sgr(s.HighlightFor(cell.Hl), cursor, s.Cursor))
			openHl = cell.Hl
			openCursor = cursor
		}
		// Write the grapheme
		b.WriteString(cell.Text)
	}
	// Close the last run
	b.WriteString(reset)
	// Return the row
	return b.String()
}

/**
 * sgr
 * Builds the escape sequence for a highlight, with the cursor drawn as reverse for a block and underline otherwise
 * @param h {Highlight} - the merged highlight
 * @param cursor {bool} - whether this cell holds the cursor
 * @param shape {CursorShape} - the cursor shape for the current mode
 * @return string
 **/
func sgr(h Highlight, cursor bool, shape CursorShape) string {
	// The parameters collected so far
	params := make([]string, 0, 8)
	// The colors to use, swapped when reversed
	fg, bg := h.Fg, h.Bg
	// Whether to draw reversed
	reverse := h.Reverse
	// A block cursor reverses the cell, any other shape underlines it
	if cursor {
		if shape == CursorBlock {
			reverse = !reverse
		} else {
			params = append(params, "4")
		}
	}
	// Apply the reverse by swapping colors when both are set and by the attribute otherwise
	if reverse {
		if fg != NoColor && bg != NoColor {
			fg, bg = bg, fg
		} else {
			params = append(params, "7")
		}
	}
	// Add the flags
	if h.Bold {
		params = append(params, "1")
	}
	if h.Italic {
		params = append(params, "3")
	}
	if h.Underline {
		params = append(params, "4")
	}
	if h.Strike {
		params = append(params, "9")
	}
	// Add the colors
	if fg != NoColor {
		params = append(params, "38;2;"+rgb(fg))
	}
	if bg != NoColor {
		params = append(params, "48;2;"+rgb(bg))
	}
	// Return nothing when there is nothing to set
	if len(params) == 0 {
		return ""
	}
	// Return the sequence
	return "\x1b[" + strings.Join(params, ";") + "m"
}

/**
 * rgb
 * Splits a 24 bit color into the r;g;b form an SGR sequence takes
 * @param c {int} - the color
 * @return string
 **/
func rgb(c int) string {
	// Pull out the three channels
	r := (c >> 16) & 0xff
	g := (c >> 8) & 0xff
	b := c & 0xff
	// Join them
	return strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b)
}
