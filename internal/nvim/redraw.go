package nvim

import "fmt"

// mainGrid is the only grid used until multigrid lands
const mainGrid = 1

/**
 * apply
 * Applies one redraw update, an event name followed by one or more arg tuples
 * @param update {[]interface{}} - the update as the redraw handler received it
 * @return error
 **/
func (s *state) apply(update []interface{}) error {
	// Fail on an empty update
	if len(update) == 0 {
		return fmt.Errorf("redraw: empty update")
	}
	// Read the event name
	name, ok := asString(update[0])
	// Fail when the name is not a string
	if !ok {
		return fmt.Errorf("redraw: event name is %T", update[0])
	}
	// Loop over every arg tuple after the name
	for _, raw := range update[1:] {
		// Read the tuple
		args, ok := asSlice(raw)
		// Fail when the tuple is not a slice
		if !ok {
			return fmt.Errorf("%s: args are %T", name, raw)
		}
		// Apply the one tuple
		if err := s.applyOne(name, args); err != nil {
			return err
		}
	}
	// Report success
	return nil
}

/**
 * applyOne
 * Applies one arg tuple of a named event, unknown events are ignored on purpose
 * @param name {string} - the event name
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) applyOne(name string, args []interface{}) error {
	// Dispatch on the event name
	switch name {
	case "grid_resize":
		return s.onGridResize(args)
	case "grid_clear":
		return s.onGridClear(args)
	case "grid_line":
		return s.onGridLine(args)
	case "grid_scroll":
		return s.onGridScroll(args)
	case "grid_cursor_goto":
		return s.onCursorGoto(args)
	case "hl_attr_define":
		return s.onHlAttrDefine(args)
	case "default_colors_set":
		return s.onDefaultColors(args)
	case "mode_info_set":
		return s.onModeInfoSet(args)
	case "mode_change":
		return s.onModeChange(args)
	case "set_title":
		return s.onSetTitle(args)
	}
	// Ignore every other event
	return nil
}

/**
 * onGridResize
 * Handles grid_resize [grid, width, height]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onGridResize(args []interface{}) error {
	// Read the grid id
	grid, err := intAt(args, 0, "grid_resize")
	if err != nil {
		return err
	}
	// Ignore every grid but the main one
	if grid != mainGrid {
		return nil
	}
	// Read the width
	width, err := intAt(args, 1, "grid_resize")
	if err != nil {
		return err
	}
	// Read the height
	height, err := intAt(args, 2, "grid_resize")
	if err != nil {
		return err
	}
	// Resize the grid
	s.resize(width, height)
	return nil
}

/**
 * onGridClear
 * Handles grid_clear [grid]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onGridClear(args []interface{}) error {
	// Read the grid id
	grid, err := intAt(args, 0, "grid_clear")
	if err != nil {
		return err
	}
	// Clear only the main grid
	if grid == mainGrid {
		s.clear()
	}
	return nil
}

/**
 * onGridLine
 * Handles grid_line [grid, row, col_start, cells, wrap] where a cell is [text, hl_id?, repeat?]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onGridLine(args []interface{}) error {
	// Read the grid id
	grid, err := intAt(args, 0, "grid_line")
	if err != nil {
		return err
	}
	// Ignore every grid but the main one
	if grid != mainGrid {
		return nil
	}
	// Read the row
	row, err := intAt(args, 1, "grid_line")
	if err != nil {
		return err
	}
	// Read the start column
	col, err := intAt(args, 2, "grid_line")
	if err != nil {
		return err
	}
	// Fail when the row is outside the grid
	if row < 0 || row >= s.height {
		return fmt.Errorf("grid_line: row %d outside height %d", row, s.height)
	}
	// Read the cell list
	cells, ok := asSlice(argOrNil(args, 3))
	if !ok {
		return fmt.Errorf("grid_line: cells are not a list")
	}
	// The highlight carried from the previous cell
	hl := 0
	// Loop over every cell
	for _, raw := range cells {
		// Read the cell tuple
		cell, ok := asSlice(raw)
		if !ok || len(cell) == 0 {
			return fmt.Errorf("grid_line: bad cell %v", raw)
		}
		// Read the text
		text, ok := asString(cell[0])
		if !ok {
			return fmt.Errorf("grid_line: cell text is %T", cell[0])
		}
		// Take a new highlight when the cell carries one
		if len(cell) > 1 {
			if n, ok := asInt(cell[1]); ok {
				hl = n
			}
		}
		// The repeat count, one unless the cell carries one
		repeat := 1
		if len(cell) > 2 {
			if n, ok := asInt(cell[2]); ok {
				repeat = n
			}
		}
		// Write the cell repeat times
		for i := 0; i < repeat && col < s.width; i++ {
			s.rows[row][col] = Cell{Text: text, Hl: hl}
			col++
		}
	}
	return nil
}

/**
 * onGridScroll
 * Handles grid_scroll [grid, top, bot, left, right, rows, cols], every dst row takes row dst+rows
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onGridScroll(args []interface{}) error {
	// The seven ints of the tuple
	var v [7]int
	// Loop over every position
	for i := range v {
		// Read the int
		n, err := intAt(args, i, "grid_scroll")
		if err != nil {
			return err
		}
		v[i] = n
	}
	// Name the parts
	grid, top, bot, left, right, rows := v[0], v[1], v[2], v[3], v[4], v[5]
	// Ignore every grid but the main one
	if grid != mainGrid {
		return nil
	}
	// Clamp the region into the grid
	top = clamp(top, 0, s.height)
	bot = clamp(bot, 0, s.height)
	left = clamp(left, 0, s.width)
	right = clamp(right, 0, s.width)
	// Copy upward when rows is positive
	if rows > 0 {
		for dst := top; dst < bot-rows; dst++ {
			copy(s.rows[dst][left:right], s.rows[dst+rows][left:right])
		}
		return nil
	}
	// Copy downward when rows is negative, walking from the bottom so no row is clobbered before it is read
	for dst := bot - 1; dst >= top-rows; dst-- {
		copy(s.rows[dst][left:right], s.rows[dst+rows][left:right])
	}
	return nil
}

/**
 * onCursorGoto
 * Handles grid_cursor_goto [grid, row, col]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onCursorGoto(args []interface{}) error {
	// Read the grid id
	grid, err := intAt(args, 0, "grid_cursor_goto")
	if err != nil {
		return err
	}
	// Ignore every grid but the main one
	if grid != mainGrid {
		return nil
	}
	// Read the row
	row, err := intAt(args, 1, "grid_cursor_goto")
	if err != nil {
		return err
	}
	// Read the column
	col, err := intAt(args, 2, "grid_cursor_goto")
	if err != nil {
		return err
	}
	// Store the cursor clamped into the grid
	s.cursorRow = clamp(row, 0, s.height-1)
	s.cursorCol = clamp(col, 0, s.width-1)
	return nil
}

/**
 * onHlAttrDefine
 * Handles hl_attr_define [id, rgb_attr, cterm_attr, info]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onHlAttrDefine(args []interface{}) error {
	// Read the id
	id, err := intAt(args, 0, "hl_attr_define")
	if err != nil {
		return err
	}
	// Read the rgb attribute map
	attr, ok := asMap(argOrNil(args, 1))
	if !ok {
		return fmt.Errorf("hl_attr_define: rgb attrs are not a map")
	}
	// Store the decoded highlight
	s.highlights[id] = decodeHighlight(attr)
	return nil
}

/**
 * decodeHighlight
 * Turns an rgb attribute map into a Highlight
 * @param attr {map[string]interface{}} - the rgb attribute map
 * @return Highlight
 **/
func decodeHighlight(attr map[string]interface{}) Highlight {
	// Start with no colors
	h := Highlight{Fg: NoColor, Bg: NoColor, Sp: NoColor}
	// Read the colors when present
	if n, ok := asInt(attr["foreground"]); ok {
		h.Fg = n
	}
	if n, ok := asInt(attr["background"]); ok {
		h.Bg = n
	}
	if n, ok := asInt(attr["special"]); ok {
		h.Sp = n
	}
	// Read the flags
	h.Bold = flag(attr, "bold")
	h.Italic = flag(attr, "italic")
	h.Strike = flag(attr, "strikethrough")
	h.Reverse = flag(attr, "reverse")
	// Any underline style counts as underlined
	h.Underline = flag(attr, "underline") || flag(attr, "undercurl") ||
		flag(attr, "underdouble") || flag(attr, "underdotted") || flag(attr, "underdashed")
	// Return the highlight
	return h
}

/**
 * onDefaultColors
 * Handles default_colors_set [rgb_fg, rgb_bg, rgb_sp, cterm_fg, cterm_bg]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onDefaultColors(args []interface{}) error {
	// Read the foreground
	fg, err := intAt(args, 0, "default_colors_set")
	if err != nil {
		return err
	}
	// Read the background
	bg, err := intAt(args, 1, "default_colors_set")
	if err != nil {
		return err
	}
	// Read the special color
	sp, err := intAt(args, 2, "default_colors_set")
	if err != nil {
		return err
	}
	// Store the defaults
	s.def = Highlight{Fg: fg, Bg: bg, Sp: sp}
	return nil
}

/**
 * onModeInfoSet
 * Handles mode_info_set [cursor_style_enabled, mode_info] keeping the cursor shape and name of every mode
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onModeInfoSet(args []interface{}) error {
	// Read the mode list
	modes, ok := asSlice(argOrNil(args, 1))
	if !ok {
		return fmt.Errorf("mode_info_set: modes are not a list")
	}
	// The new shape and name tables
	shapes := make([]CursorShape, 0, len(modes))
	names := make([]string, 0, len(modes))
	// Loop over every mode
	for _, raw := range modes {
		// Read the mode map
		m, ok := asMap(raw)
		if !ok {
			return fmt.Errorf("mode_info_set: mode is not a map")
		}
		// Read the cursor shape name
		shape, _ := asString(m["cursor_shape"])
		// Read the mode name
		name, _ := asString(m["name"])
		// Append the decoded shape and the name
		shapes = append(shapes, decodeShape(shape))
		names = append(names, name)
	}
	// Store the tables
	s.modeShapes = shapes
	s.modeNames = names
	return nil
}

/**
 * decodeShape
 * Turns a cursor_shape name into a CursorShape
 * @param name {string} - block, horizontal, or vertical
 * @return CursorShape
 **/
func decodeShape(name string) CursorShape {
	// Map the two non block names
	switch name {
	case "horizontal":
		return CursorHorizontal
	case "vertical":
		return CursorVertical
	}
	// Everything else is a block
	return CursorBlock
}

/**
 * onModeChange
 * Handles mode_change [mode, mode_idx]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onModeChange(args []interface{}) error {
	// Read the mode name
	mode, err := stringAt(args, 0, "mode_change")
	if err != nil {
		return err
	}
	// Read the mode index
	idx, err := intAt(args, 1, "mode_change")
	if err != nil {
		return err
	}
	// Store both
	s.mode = mode
	s.modeIdx = idx
	return nil
}

/**
 * onSetTitle
 * Handles set_title [title]
 * @param args {[]interface{}} - the arg tuple
 * @return error
 **/
func (s *state) onSetTitle(args []interface{}) error {
	// Read the title
	title, err := stringAt(args, 0, "set_title")
	if err != nil {
		return err
	}
	// Store it
	s.title = title
	return nil
}

/**
 * flag
 * Reads a bool flag from an attribute map
 * @param attr {map[string]interface{}} - the map
 * @param key {string} - the flag name
 * @return bool
 **/
func flag(attr map[string]interface{}, key string) bool {
	// Read the value as a bool
	b, _ := attr[key].(bool)
	// Return it
	return b
}

/**
 * argOrNil
 * Returns the arg at an index or nil when the tuple is short
 * @param args {[]interface{}} - the arg tuple
 * @param i {int} - the index
 * @return interface{}
 **/
func argOrNil(args []interface{}, i int) interface{} {
	// Return nil past the end
	if i >= len(args) {
		return nil
	}
	// Return the arg
	return args[i]
}
