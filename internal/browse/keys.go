package browse

import "github.com/charmbracelet/bubbles/key"

// keyMap centralizes every binding the browse TUI uses. Each binding carries
// its own help text, so help.Model renders a consistent footer without per-
// screen string lists. j/k are NOT bound to Up/Down because the persistent
// textinput on screenBrowse needs those keys for typing.
type keyMap struct {
	Up, Down         key.Binding
	PageUp, PageDown key.Binding
	Tab, ShiftTab    key.Binding
	Enter, Back      key.Binding
	Filter           key.Binding // display-only; list.Model owns the /-handling on drilldown
	Help             key.Binding
	Quit             key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:     key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^C", "quit")),
	}
}

// screenKeys adapts keyMap to bubbles' help.KeyMap interface, with bindings
// specialized to the current screen.
type screenKeys struct {
	k keyMap
	s screen
}

func (sk screenKeys) ShortHelp() []key.Binding {
	switch sk.s {
	case screenBrowse:
		return []key.Binding{sk.k.Up, sk.k.Down, sk.k.Tab, sk.k.Enter, sk.k.Back, sk.k.Help, sk.k.Quit}
	case screenDrilldown:
		return []key.Binding{sk.k.Up, sk.k.Down, sk.k.Enter, sk.k.Back, sk.k.Filter, sk.k.Help, sk.k.Quit}
	}
	return nil
}

func (sk screenKeys) FullHelp() [][]key.Binding {
	nav := []key.Binding{sk.k.Up, sk.k.Down, sk.k.PageUp, sk.k.PageDown}
	switch sk.s {
	case screenBrowse:
		return [][]key.Binding{
			nav,
			{sk.k.Tab, sk.k.ShiftTab},
			{sk.k.Enter, sk.k.Back},
			{sk.k.Help, sk.k.Quit},
		}
	case screenDrilldown:
		return [][]key.Binding{
			nav,
			{sk.k.Enter, sk.k.Back, sk.k.Filter},
			{sk.k.Help, sk.k.Quit},
		}
	}
	return nil
}
