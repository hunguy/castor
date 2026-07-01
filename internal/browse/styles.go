package browse

import "github.com/charmbracelet/lipgloss"

// Monochrome palette. Zero chroma — hierarchy comes entirely from
// brightness and weight. The "accent" is pure white; everything else is
// a step down the grayscale ramp (xterm 256-color 232-255 is grayscale,
// plus 231 = pure white).
var (
	accent      = lipgloss.Color("231") // pure white — active title, selected row, prompt, active dot
	fgPrimary   = lipgloss.Color("252") // body text — list titles, overview
	fgSecondary = lipgloss.Color("247") // hierarchy — meta title, selected description
	fgMuted     = lipgloss.Color("242") // captions, help, inactive dots, separators
	errorColor  = lipgloss.Color("231") // errors use weight + underline, not hue
)

// Spacing scale — terminal cells. Two values cover every horizontal inset
// in the TUI; verticals are always a single blank row between sections.
const (
	spInline = 2 // title pad, prompt width, list-item indent, status separator
	spGutter = 2 // between list and poster panel
)

// styles holds every lipgloss.Style the browse TUI uses. Centralized so
// the View code reads as composition, and so palette changes happen in
// one place.
type styles struct {
	Title     lipgloss.Style // H1: bold white + horizontal inset
	TitleText lipgloss.Style // H1 text only — no padding, for multi-part headers
	MetaTitle lipgloss.Style // H2: secondary foreground, no bold
	Muted     lipgloss.Style // captions, hints, separators
	Overview  lipgloss.Style // body text in poster panel (clamped width)
	Err       lipgloss.Style
}

func newStyles() styles {
	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(accent)

	return styles{
		Title:     titleText.Padding(0, spInline),
		TitleText: titleText,
		MetaTitle: lipgloss.NewStyle().Foreground(fgSecondary),
		Muted:     lipgloss.NewStyle().Foreground(fgMuted),
		Overview: lipgloss.NewStyle().
			Foreground(fgPrimary).
			Width(posterCols),
		Err: lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			Underline(true).
			Padding(0, spInline),
	}
}

// headerPad wraps already-styled inner content with the horizontal inset
// shared by Title — used when a header is composed of multiple styled
// parts (e.g. accent-colored words separated by muted › glyphs) and we
// can't just call Title.Render on the whole string.
func headerPad(s string) string {
	return lipgloss.NewStyle().Padding(0, spInline).Render(s)
}
