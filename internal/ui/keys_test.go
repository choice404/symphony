package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTranslateTable(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyMsg
		want string
	}{
		{"letter", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, "a"},
		{"word", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")}, "hi"},
		{"less than", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<")}, "<lt>"},
		{"less than in text", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a<b")}, "a<lt>b"},
		{"alt rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, "<M-x>"},
		{"alt less than", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("<"), Alt: true}, "<M-lt>"},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, "<CR>"},
		{"esc", tea.KeyMsg{Type: tea.KeyEscape}, "<Esc>"},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, "<BS>"},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, "<Tab>"},
		{"shift tab", tea.KeyMsg{Type: tea.KeyShiftTab}, "<S-Tab>"},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, "<Space>"},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, "<Up>"},
		{"ctrl up", tea.KeyMsg{Type: tea.KeyCtrlUp}, "<C-Up>"},
		{"ctrl shift up", tea.KeyMsg{Type: tea.KeyCtrlShiftUp}, "<C-S-Up>"},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, "<PageUp>"},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, "<PageDown>"},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, "<Del>"},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, "<Home>"},
		{"ctrl home", tea.KeyMsg{Type: tea.KeyCtrlHome}, "<C-Home>"},
		{"f1", tea.KeyMsg{Type: tea.KeyF1}, "<F1>"},
		{"f12", tea.KeyMsg{Type: tea.KeyF12}, "<F12>"},
		{"ctrl a", tea.KeyMsg{Type: tea.KeyCtrlA}, "<C-a>"},
		{"ctrl c", tea.KeyMsg{Type: tea.KeyCtrlC}, "<C-c>"},
		{"ctrl at", tea.KeyMsg{Type: tea.KeyCtrlAt}, "<C-@>"},
		{"ctrl backslash", tea.KeyMsg{Type: tea.KeyCtrlBackslash}, "<C-\\>"},
		{"ctrl close bracket", tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, "<C-]>"},
		{"ctrl caret", tea.KeyMsg{Type: tea.KeyCtrlCaret}, "<C-^>"},
		{"ctrl underscore", tea.KeyMsg{Type: tea.KeyCtrlUnderscore}, "<C-_>"},
		{"alt enter", tea.KeyMsg{Type: tea.KeyEnter, Alt: true}, "<M-CR>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Translate(c.msg)
			if got.Keys != c.want || got.Paste != "" {
				t.Fatalf("Translate(%s) = %+v want keys %q", c.msg, got, c.want)
			}
		})
	}
}

func TestTranslatePaste(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line one\nline <two>"), Paste: true}
	got := Translate(msg)
	if got.Keys != "" || got.Paste != "line one\nline <two>" {
		t.Fatalf("paste = %+v", got)
	}
}
