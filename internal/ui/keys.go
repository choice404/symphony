package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Input is what one key event becomes on the way to nvim, Keys in key notation or Paste as raw text
type Input struct {
	// The keys in nvim key notation, empty for a paste
	Keys string
	// The pasted text, empty for a key
	Paste string
}

// baseNames maps a bubbletea key name to the nvim key notation name
var baseNames = map[string]string{
	"enter":     "CR",
	"esc":       "Esc",
	"backspace": "BS",
	"tab":       "Tab",
	" ":         "Space",
	"up":        "Up",
	"down":      "Down",
	"left":      "Left",
	"right":     "Right",
	"home":      "Home",
	"end":       "End",
	"pgup":      "PageUp",
	"pgdown":    "PageDown",
	"delete":    "Del",
	"insert":    "Insert",
}

/**
 * Translate
 * Turns a bubbletea key event into the input nvim expects
 * @param msg {tea.KeyMsg} - the key event
 * @return Input
 **/
func Translate(msg tea.KeyMsg) Input {
	// Plain text is either a paste or one or more typed runes
	if msg.Type == tea.KeyRunes {
		if msg.Paste {
			return Input{Paste: string(msg.Runes)}
		}
		return Input{Keys: translateRunes(msg.Runes, msg.Alt)}
	}
	// Every other key has a name, read it without the alt prefix
	name := tea.Key{Type: msg.Type}.String()
	// Split the name into modifiers and the base key
	mods, base := splitName(name)
	// Add alt when the event carries it
	if msg.Alt {
		mods = append(mods, "M")
	}
	// Return the notation
	return Input{Keys: notation(mods, baseName(base))}
}

/**
 * translateRunes
 * Turns typed runes into notation, escaping < and wrapping each rune with alt when set
 * @param runes {[]rune} - the typed runes
 * @param alt {bool} - whether alt was held
 * @return string
 **/
func translateRunes(runes []rune, alt bool) string {
	// The notation collected so far
	var b strings.Builder
	// Loop over every rune
	for _, r := range runes {
		// The rune as nvim spells it, < must be written as <lt>
		text := string(r)
		if r == '<' {
			text = "lt"
		}
		// Wrap in alt when held, or in brackets when the rune needed escaping
		switch {
		case alt:
			b.WriteString("<M-" + text + ">")
		case r == '<':
			b.WriteString("<lt>")
		default:
			b.WriteString(text)
		}
	}
	// Return the notation
	return b.String()
}

/**
 * splitName
 * Splits a bubbletea key name such as ctrl+shift+up into modifier letters and the base
 * @param name {string} - the key name
 * @return []string, string
 **/
func splitName(name string) ([]string, string) {
	// A lone space has no modifiers
	if name == " " {
		return nil, " "
	}
	// Split on plus
	parts := strings.Split(name, "+")
	// The base is the last part
	base := parts[len(parts)-1]
	// The modifiers collected so far
	mods := make([]string, 0, 3)
	// Loop over every part before the base
	for _, p := range parts[:len(parts)-1] {
		// Map the modifier word to its letter
		switch p {
		case "ctrl":
			mods = append(mods, "C")
		case "alt":
			mods = append(mods, "M")
		case "shift":
			mods = append(mods, "S")
		}
	}
	// Return both
	return mods, base
}

/**
 * baseName
 * Maps a base key name to its nvim spelling, f keys upper cased and single chars kept
 * @param base {string} - the bubbletea base name
 * @return string
 **/
func baseName(base string) string {
	// Use the table when the name is in it
	if n, ok := baseNames[base]; ok {
		return n
	}
	// Upper case the f of a function key
	if len(base) > 1 && base[0] == 'f' {
		return "F" + base[1:]
	}
	// Keep everything else, a letter or a symbol after ctrl
	return base
}

/**
 * notation
 * Joins modifiers and a base into <C-M-S-Base>, or the bare base when there are no modifiers and the base is one plain char
 * @param mods {[]string} - the modifier letters
 * @param base {string} - the nvim base name
 * @return string
 **/
func notation(mods []string, base string) string {
	// A bare single char with no modifiers is sent as itself
	if len(mods) == 0 && len(base) == 1 && base != "<" {
		return base
	}
	// The prefix of modifiers
	prefix := ""
	for _, m := range mods {
		prefix += m + "-"
	}
	// Return the bracketed form
	return "<" + prefix + base + ">"
}
