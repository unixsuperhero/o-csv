// keys_test.go (copied from ochazuke internal/keytest, which o-csv cannot import) drives Bubble Tea models from tests using the key names that
// bindings use ("ctrl+c", "esc", "enter", "shift+tab", "alt+b", "space", "x").
// Msg is the only place the test suite spells out a Bubble Tea key message, so
// a Bubble Tea upgrade changes this file and nothing else.
package main

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Model is the part of a Bubble Tea model that receives keys.
type Model interface {
	Update(tea.Msg) (tea.Model, tea.Cmd)
}

// Msg builds the Bubble Tea v2 message for one key. Paste text becomes a
// tea.PasteMsg. Otherwise leading "ctrl+", "alt+", "shift+" (and the other
// modifier names Bubble Tea prints) set Mod; a name Bubble Tea knows ("esc",
// "space", "pgup") becomes that key code; a single rune is a printable key
// whose Text is set when no modifier is held. Literal text skips the name
// lookup, so Msg("ctrl+s", true, false) is the six-character text "ctrl+s".
// Bubble Tea v2 never coalesces typed runes into one key press, so Type and
// Press split multi-rune text into one message per rune instead.
func Msg(name string, literal, paste bool) tea.Msg {
	if paste {
		return tea.PasteMsg{Content: name}
	}
	if literal {
		return tea.KeyPressMsg{Code: firstRune(name), Text: name}
	}
	var mod tea.KeyMod
	for {
		prefix, rest, ok := strings.Cut(name, "+")
		m, known := modifiers[prefix]
		if !ok || !known || rest == "" {
			break
		}
		mod, name = mod|m, rest
	}
	if code, ok := keyCodes[name]; ok {
		return tea.KeyPressMsg{Code: code, Mod: mod}
	}
	msg := tea.KeyPressMsg{Code: firstRune(name), Mod: mod}
	if mod == 0 {
		msg.Text = name
	}
	return msg
}

// Press sends one named key, such as "ctrl+x", "esc" or "q", and returns the
// command the model produced. Unknown multi-rune text is typed rune by rune.
func Press(m Model, name string) tea.Cmd {
	if _, known := keyCodes[name]; !known && !strings.Contains(name, "+") && utf8.RuneCountInString(name) > 1 {
		return Type(m, name)
	}
	_, cmd := m.Update(Msg(name, false, false))
	return cmd
}

// Type sends text as single-rune literal key presses, the way Bubble Tea v2
// delivers typing, and batches the commands they produce.
func Type(m Model, text string) tea.Cmd {
	var commands []tea.Cmd
	for _, r := range text {
		if _, cmd := m.Update(Msg(string(r), true, false)); cmd != nil {
			commands = append(commands, cmd)
		}
	}
	return tea.Batch(commands...)
}

// Paste sends text as one bracketed paste event.
func Paste(m Model, text string) tea.Cmd {
	_, cmd := m.Update(Msg(text, false, true))
	return cmd
}

func firstRune(s string) rune {
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

var modifiers = map[string]tea.KeyMod{
	"ctrl":  tea.ModCtrl,
	"alt":   tea.ModAlt,
	"shift": tea.ModShift,
	"meta":  tea.ModMeta,
	"hyper": tea.ModHyper,
	"super": tea.ModSuper,
}

// keyCodes maps every name Bubble Tea v2 prints for a special key back to its
// code, derived from tea.Key.String so the names match what Model.Update sees.
// Special codes are the control characters and the range above KeyExtended.
var keyCodes = func() map[string]rune {
	names := map[string]rune{}
	add := func(code rune) {
		s := (tea.Key{Code: code}).String()
		if utf8.RuneCountInString(s) > 1 && !strings.Contains(s, "+") {
			names[s] = code
		}
	}
	for code := rune(0); code < 128; code++ {
		add(code)
	}
	for code := tea.KeyExtended; code <= tea.KeyExtended+1024; code++ {
		add(code)
	}
	return names
}()
