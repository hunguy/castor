// Package browse is a Bubble Tea TUI for searching TMDB and picking a
// movie or TV episode to cast. It does not touch the cast pipeline — Run
// returns the user's Selection and the caller hands it off.
package browse

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stupside/castor/internal/browse/tmdb"
)

// ---------------------------------------------------------------- public API

type Kind int

const (
	KindNone Kind = iota
	KindMovie
	KindEpisode
)

type Selection struct {
	Kind    Kind
	TMDBID  string
	Title   string
	Season  uint
	Episode uint
}

// Run blocks on the TUI until the user picks or quits.
func Run(ctx context.Context, client *tmdb.Client) (Selection, error) {
	m := newModel(ctx, client)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return Selection{}, err
	}
	if fm, ok := final.(model); ok {
		return fm.sel, nil
	}
	return Selection{}, nil
}

// ---------------------------------------------------------------- constants

const (
	// Poster footprint in terminal cells. Half-block rendering means each
	// cell shows 2 stacked pixels vertically, so the rendered pixel grid is
	// posterCols × (posterRows*2). 27 × 40 ≈ 2:3 — the canonical movie-poster
	// aspect ratio. Sizing the box correctly stops pixterm from stretching
	// the image horizontally (the "fat face" look in earlier screenshots).
	posterCols     = 27
	posterRows     = 20
	searchDebounce = 250 * time.Millisecond
)

// ---------------------------------------------------------------- screens

type screen int

const (
	screenBrowse screen = iota
	screenDrilldown
)

type drillMode int

const (
	modeSeasons drillMode = iota
	modeEpisodes
)

// ---------------------------------------------------------------- tabs

type tabID int

const (
	tabTrending tabID = iota
	tabPopularMovies
	tabTopMovies
	tabPopularTV
	tabTopTV
	tabCount
)

func (t tabID) label() string {
	return [...]string{"Trending", "Popular Movies", "Top Movies", "Popular TV", "Top TV"}[t]
}

// ---------------------------------------------------------------- list items

type resultItem struct{ r tmdb.SearchResult }

func (i resultItem) Title() string {
	if y := i.r.Year(); y != "" {
		return fmt.Sprintf("%s (%s)", i.r.DisplayTitle(), y)
	}
	return i.r.DisplayTitle()
}

func (i resultItem) Description() string {
	typ := "Movie"
	if i.r.MediaType == "tv" {
		typ = "TV"
	}
	o := i.r.Overview
	if o == "" {
		return typ
	}
	if len(o) > 80 {
		o = o[:77] + "..."
	}
	return typ + " · " + o
}

func (i resultItem) FilterValue() string { return i.r.DisplayTitle() }

type seasonItem struct{ s tmdb.Season }

func (i seasonItem) Title() string {
	if i.s.Name != "" {
		return fmt.Sprintf("S%02d — %s", i.s.SeasonNumber, i.s.Name)
	}
	return fmt.Sprintf("Season %d", i.s.SeasonNumber)
}

func (i seasonItem) Description() string {
	return fmt.Sprintf("%d episodes  %s", i.s.EpisodeCount, i.s.AirDate)
}

func (i seasonItem) FilterValue() string { return i.Title() }

type episodeItem struct{ e tmdb.Episode }

func (i episodeItem) Title() string {
	return fmt.Sprintf("E%02d — %s", i.e.EpisodeNumber, i.e.Name)
}

func (i episodeItem) Description() string {
	if i.e.Overview == "" {
		return i.e.AirDate
	}
	if len(i.e.Overview) > 120 {
		return i.e.Overview[:117] + "..."
	}
	return i.e.Overview
}

func (i episodeItem) FilterValue() string { return i.e.Name }

// ---------------------------------------------------------------- model

type model struct {
	ctx    context.Context
	client *tmdb.Client

	styles styles
	keys   keyMap
	help   help.Model

	scr screen

	// browse screen
	tab        tabID
	query      textinput.Model
	queryTok   int // monotonic; only the latest debounce tick fires
	browse     list.Model
	topsCache  [tabCount][]list.Item
	topsLoaded [tabCount]bool
	topsCursor [tabCount]int

	// drilldown screen
	drillMode    drillMode
	drill        list.Model
	tvID         int
	tvName       string
	seasonNum    int
	seasonsCache []list.Item // restore on episodes → seasons back

	// shared
	spin    spinner.Model
	loading bool
	err     error

	posters       map[string]string
	posterPending string

	sel  Selection
	w, h int
}

func newDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(fgPrimary)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(fgMuted)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(accent).
		BorderForeground(accent).
		Bold(true)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(fgSecondary).
		BorderForeground(accent)
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Foreground(fgMuted)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(fgMuted)
	d.Styles.FilterMatch = lipgloss.NewStyle().Foreground(accent).Underline(true)
	return d
}

func newHelp() help.Model {
	h := help.New()
	h.ShowAll = false
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(fgSecondary)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(fgMuted)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(fgMuted)
	h.Styles.FullKey = h.Styles.FullKey.Foreground(fgSecondary)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(fgMuted)
	h.Styles.FullSeparator = h.Styles.FullSeparator.Foreground(fgMuted)
	h.Styles.Ellipsis = h.Styles.Ellipsis.Foreground(fgMuted)
	return h
}

func newModel(ctx context.Context, client *tmdb.Client) model {
	st := newStyles()

	q := textinput.New()
	q.Placeholder = "Type to search TMDB…"
	q.Prompt = "❯ "
	q.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	q.PlaceholderStyle = lipgloss.NewStyle().Foreground(fgMuted)
	q.TextStyle = lipgloss.NewStyle().Foreground(fgPrimary)
	q.CharLimit = 128
	q.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot // smaller, more refined than the braille Dot
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	delegate := newDelegate()

	browse := list.New(nil, delegate, 0, 0)
	browse.SetShowTitle(false)
	browse.SetShowStatusBar(false)
	browse.SetShowHelp(false)
	browse.SetFilteringEnabled(false) // textinput owns filtering on browse

	drill := list.New(nil, delegate, 0, 0)
	drill.SetShowTitle(false)
	drill.SetShowStatusBar(false)
	drill.SetShowHelp(false)
	drill.SetFilteringEnabled(true)

	return model{
		ctx:     ctx,
		client:  client,
		styles:  st,
		keys:    defaultKeys(),
		help:    newHelp(),
		scr:     screenBrowse,
		tab:     tabTrending,
		query:   q,
		browse:  browse,
		drill:   drill,
		spin:    sp,
		posters: map[string]string{},
		loading: true,
	}
}

// ---------------------------------------------------------------- messages

type topsLoadedMsg struct {
	tab tabID
	res []tmdb.SearchResult
	err error
}

type searchTickMsg struct {
	tok   int
	query string
}

type searchDoneMsg struct {
	tok int
	res []tmdb.SearchResult
	err error
}

type tvDoneMsg struct {
	tv  *tmdb.TVDetails
	err error
}

type seasonDoneMsg struct {
	sd  *tmdb.SeasonDetails
	err error
}

func loadTopCmd(ctx context.Context, c *tmdb.Client, t tabID) tea.Cmd {
	return func() tea.Msg {
		var (
			res []tmdb.SearchResult
			err error
		)
		switch t {
		case tabTrending:
			res, err = c.Trending(ctx)
		case tabPopularMovies:
			res, err = c.PopularMovies(ctx)
		case tabTopMovies:
			res, err = c.TopRatedMovies(ctx)
		case tabPopularTV:
			res, err = c.PopularTV(ctx)
		case tabTopTV:
			res, err = c.TopRatedTV(ctx)
		}
		return topsLoadedMsg{tab: t, res: res, err: err}
	}
}

func searchTickCmd(tok int, query string) tea.Cmd {
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{tok: tok, query: query}
	})
}

func searchCmd(ctx context.Context, c *tmdb.Client, tok int, q string) tea.Cmd {
	return func() tea.Msg {
		res, err := c.SearchMulti(ctx, q)
		return searchDoneMsg{tok: tok, res: res, err: err}
	}
}

func tvCmd(ctx context.Context, c *tmdb.Client, id int) tea.Cmd {
	return func() tea.Msg {
		tv, err := c.TV(ctx, id)
		return tvDoneMsg{tv: tv, err: err}
	}
}

func seasonCmd(ctx context.Context, c *tmdb.Client, tvID, n int) tea.Cmd {
	return func() tea.Msg {
		sd, err := c.Season(ctx, tvID, n)
		return seasonDoneMsg{sd: sd, err: err}
	}
}

// ---------------------------------------------------------------- tea.Model

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, textinput.Blink, loadTopCmd(m.ctx, m.client, m.tab))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.resize()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			if !m.drillInFilter() {
				m.help.ShowAll = !m.help.ShowAll
				m.resize()
				return m, nil
			}
		}

	case topsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := toResultItems(msg.res)
		m.topsCache[msg.tab] = items
		m.topsLoaded[msg.tab] = true
		if m.scr == screenBrowse && m.tab == msg.tab && m.query.Value() == "" {
			m.browse.SetItems(items)
			m.browse.Select(m.topsCursor[msg.tab])
			return m, m.maybeFetchPosterFor(m.browse)
		}
		return m, nil

	case searchTickMsg:
		if msg.tok != m.queryTok {
			return m, nil // stale; a newer keystroke supersedes this tick
		}
		if msg.query == "" {
			m.applyTab()
			return m, m.maybeFetchPosterFor(m.browse)
		}
		m.loading = true
		m.err = nil
		return m, tea.Batch(searchCmd(m.ctx, m.client, msg.tok, msg.query), m.spin.Tick)

	case searchDoneMsg:
		if msg.tok != m.queryTok {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.browse.SetItems(toResultItems(msg.res))
		m.browse.Select(0)
		return m, m.maybeFetchPosterFor(m.browse)

	case tvDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.tvName = msg.tv.Name
		items := make([]list.Item, 0, len(msg.tv.Seasons))
		for _, s := range msg.tv.Seasons {
			if s.EpisodeCount == 0 {
				continue
			}
			items = append(items, seasonItem{s: s})
		}
		m.seasonsCache = items
		m.drill.SetItems(items)
		m.drill.Select(0)
		m.drillMode = modeSeasons
		m.scr = screenDrilldown
		return m, nil

	case seasonDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		items := make([]list.Item, 0, len(msg.sd.Episodes))
		for _, e := range msg.sd.Episodes {
			items = append(items, episodeItem{e: e})
		}
		m.drill.SetItems(items)
		m.drill.Select(0)
		m.drillMode = modeEpisodes
		return m, nil

	case posterReadyMsg:
		if msg.err == nil && msg.ansi != "" {
			m.posters[msg.posterPath] = msg.ansi
		}
		if msg.posterPath == m.posterPending {
			m.posterPending = ""
		}
		return m, nil
	}

	switch m.scr {
	case screenBrowse:
		return m.updateBrowse(msg)
	case screenDrilldown:
		return m.updateDrilldown(msg)
	}
	return m, nil
}

// ---------------------------------------------------------------- browse update

func (m model) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, m.keys.Tab):
			if m.query.Value() != "" {
				return m, nil // tab is meaningless while searching
			}
			return m.cycleTab(1)
		case key.Matches(km, m.keys.ShiftTab):
			if m.query.Value() != "" {
				return m, nil
			}
			return m.cycleTab(-1)
		case key.Matches(km, m.keys.Enter):
			if it, ok := m.browse.SelectedItem().(resultItem); ok {
				return m.pickResult(it.r)
			}
			return m, nil
		case key.Matches(km, m.keys.Back):
			if m.query.Value() != "" {
				m.query.SetValue("")
				m.queryTok++
				m.applyTab()
				return m, m.maybeFetchPosterFor(m.browse)
			}
			return m, nil
		case key.Matches(km, m.keys.Up),
			key.Matches(km, m.keys.Down),
			key.Matches(km, m.keys.PageUp),
			key.Matches(km, m.keys.PageDown):
			return m.delegateBrowseList(msg)
		}
	}
	// Anything else: forward to the textinput. If the query changed, kick
	// a debounced search tick.
	prev := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() != prev {
		m.queryTok++
		return m, tea.Batch(cmd, searchTickCmd(m.queryTok, m.query.Value()))
	}
	return m, cmd
}

func (m model) cycleTab(delta int) (tea.Model, tea.Cmd) {
	m.topsCursor[m.tab] = m.browse.Index()
	n := int(tabCount)
	m.tab = tabID(((int(m.tab)+delta)%n + n) % n)
	return m, m.ensureTabLoaded()
}

func (m *model) applyTab() {
	m.browse.SetItems(m.topsCache[m.tab])
	if m.topsCursor[m.tab] < len(m.topsCache[m.tab]) {
		m.browse.Select(m.topsCursor[m.tab])
	}
}

func (m *model) ensureTabLoaded() tea.Cmd {
	if m.topsLoaded[m.tab] {
		m.applyTab()
		return m.maybeFetchPosterFor(m.browse)
	}
	m.loading = true
	m.err = nil
	return tea.Batch(loadTopCmd(m.ctx, m.client, m.tab), m.spin.Tick)
}

func (m model) delegateBrowseList(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.browse.Index()
	var cmd tea.Cmd
	m.browse, cmd = m.browse.Update(msg)
	cmds := []tea.Cmd{cmd}
	if m.browse.Index() != prev {
		if c := m.maybeFetchPosterFor(m.browse); c != nil {
			cmds = append(cmds, c)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m model) pickResult(r tmdb.SearchResult) (tea.Model, tea.Cmd) {
	switch r.MediaType {
	case "movie":
		m.sel = Selection{
			Kind:   KindMovie,
			TMDBID: strconv.Itoa(r.ID),
			Title:  r.DisplayTitle(),
		}
		return m, tea.Quit
	case "tv":
		m.tvID = r.ID
		m.tvName = r.DisplayTitle()
		m.loading = true
		m.err = nil
		return m, tea.Batch(tvCmd(m.ctx, m.client, r.ID), m.spin.Tick)
	}
	return m, nil
}

// ---------------------------------------------------------------- drilldown update

func (m model) updateDrilldown(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && !m.drill.SettingFilter() {
		switch {
		case key.Matches(km, m.keys.Back):
			switch m.drillMode {
			case modeEpisodes:
				m.drill.SetItems(m.seasonsCache)
				m.drill.Select(0)
				m.drillMode = modeSeasons
				return m, nil
			case modeSeasons:
				m.scr = screenBrowse
				return m, nil
			}
		case key.Matches(km, m.keys.Enter):
			return m.drillEnter()
		}
	}
	var cmd tea.Cmd
	m.drill, cmd = m.drill.Update(msg)
	return m, cmd
}

func (m model) drillEnter() (tea.Model, tea.Cmd) {
	switch m.drillMode {
	case modeSeasons:
		if it, ok := m.drill.SelectedItem().(seasonItem); ok {
			m.seasonNum = it.s.SeasonNumber
			m.loading = true
			m.err = nil
			return m, tea.Batch(seasonCmd(m.ctx, m.client, m.tvID, it.s.SeasonNumber), m.spin.Tick)
		}
	case modeEpisodes:
		if it, ok := m.drill.SelectedItem().(episodeItem); ok {
			m.sel = Selection{
				Kind:    KindEpisode,
				TMDBID:  strconv.Itoa(m.tvID),
				Title:   fmt.Sprintf("%s · S%02dE%02d — %s", m.tvName, m.seasonNum, it.e.EpisodeNumber, it.e.Name),
				Season:  uint(m.seasonNum),
				Episode: uint(it.e.EpisodeNumber),
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) drillInFilter() bool {
	return m.scr == screenDrilldown && m.drill.SettingFilter()
}

// ---------------------------------------------------------------- poster fetch

func (m *model) maybeFetchPosterFor(l list.Model) tea.Cmd {
	it, ok := l.SelectedItem().(resultItem)
	if !ok || it.r.PosterPath == "" {
		return nil
	}
	if _, ok := m.posters[it.r.PosterPath]; ok {
		return nil
	}
	if m.posterPending == it.r.PosterPath {
		return nil
	}
	m.posterPending = it.r.PosterPath
	return fetchPosterCmd(m.ctx, it.r.PosterURL("w500"), it.r.PosterPath, posterCols, posterRows)
}

// ---------------------------------------------------------------- layout

// chrome rows reserved outside the body — header (1) + a blank between
// header and body (1) + a blank between body and footer (1). Browse also
// reserves the query row.
func (m model) bodyHeight() int {
	footerH := lipgloss.Height(m.footer())
	chrome := 3 // drilldown: header + 2 blanks
	if m.scr == screenBrowse {
		chrome = 4 // browse: header + query + 2 blanks
	}
	return max(m.h-footerH-chrome, 8)
}

func (m *model) resize() {
	h := m.bodyHeight()
	wList := max(m.w-posterCols-spGutter, 30)
	m.browse.SetSize(wList, h)
	m.drill.SetSize(max(m.w, 30), h)
	m.query.Width = max(m.w-spInline*2, 20)
}

// ---------------------------------------------------------------- view

func (m model) View() string {
	switch m.scr {
	case screenDrilldown:
		return m.viewDrilldown()
	default:
		return m.viewBrowse()
	}
}

func (m model) viewBrowse() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.browseHeader(),
		m.query.View(),
		"",
		m.bodyRow(m.browse),
		"",
		m.footer(),
	)
}

// browseHeader puts the active label flush-left and the tab indicator
// dots flush-right via lipgloss.PlaceHorizontal — no manual gap arithmetic.
// When searching, we drop the query echo from the title because the
// textinput right below already shows it.
func (m model) browseHeader() string {
	label := m.tab.label()
	if m.query.Value() != "" {
		label = "Search"
	}
	title := m.styles.Title.Render(label)
	dots := m.renderTabDots()
	rhs := lipgloss.PlaceHorizontal(max(m.w-lipgloss.Width(title), 0), lipgloss.Right, dots)
	return lipgloss.JoinHorizontal(lipgloss.Top, title, rhs)
}

func (m model) renderTabDots() string {
	activeStyle := lipgloss.NewStyle().Foreground(accent)
	inactiveStyle := lipgloss.NewStyle().Foreground(fgMuted)
	parts := make([]string, 0, int(tabCount))
	for i := range tabCount {
		glyph := "○"
		st := inactiveStyle
		if i == m.tab && m.query.Value() == "" {
			glyph = "●"
			st = activeStyle
		}
		parts = append(parts, st.Render(glyph))
	}
	return strings.Join(parts, " ")
}

func (m model) viewDrilldown() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.drillHeader(),
		"",
		m.drill.View(),
		"",
		m.footer(),
	)
}

func (m model) drillHeader() string {
	sep := m.styles.Muted.Render(" › ")
	parts := []string{m.styles.TitleText.Render(m.tvName)}
	switch m.drillMode {
	case modeSeasons:
		parts = append(parts, sep, m.styles.TitleText.Render("Seasons"))
	case modeEpisodes:
		parts = append(parts,
			sep, m.styles.TitleText.Render(fmt.Sprintf("S%02d", m.seasonNum)),
			sep, m.styles.TitleText.Render("Episodes"),
		)
	}
	return headerPad(strings.Join(parts, ""))
}

// bodyRow renders the left list + right poster panel as a single fixed-
// height row.
func (m model) bodyRow(l list.Model) string {
	left := l.View()
	right := m.posterPanel(l, m.bodyHeight())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", spGutter), right)
}

// posterPanel renders the selected item's poster + metadata, clamped to
// exactly totalH rows. The poster string is either a stream of per-cell
// true-color ANSI half-block escapes (pixterm fallback) or a single
// Kitty/iTerm/Sixel image escape sequence padded to posterRows lines —
// in BOTH cases we deliberately do NOT pass it through lipgloss's
// Width/Height/Render path: lipgloss rewrites ANSI runs and would strip
// per-pixel colour codes from pixterm, and it would (worse) split the
// inline-image control sequence and corrupt it.
func (m model) posterPanel(l list.Model, totalH int) string {
	it, ok := l.SelectedItem().(resultItem)
	if !ok {
		return blankRect(posterCols, totalH)
	}
	r := it.r

	var poster string
	if ansi, ok := m.posters[r.PosterPath]; ok && r.PosterPath != "" {
		poster = ansi
	} else {
		poster = blankRect(posterCols, posterRows)
	}

	titleLine := r.DisplayTitle()
	if y := r.Year(); y != "" {
		titleLine = fmt.Sprintf("%s (%s)", titleLine, y)
	}
	// Truncate title to the poster panel width — otherwise long titles
	// like "The Punisher: One Last Kill (2026)" overflow the right column
	// and push the bodyRow layout sideways.
	if runes := []rune(titleLine); len(runes) > posterCols {
		titleLine = string(runes[:posterCols-1]) + "…"
	}
	rating := ""
	if r.VoteAverage > 0 {
		rating = fmt.Sprintf("★ %.1f", r.VoteAverage)
	}

	const metaFixedRows = 4 // blank + title + rating + blank
	overviewH := max(totalH-posterRows-metaFixedRows, 0)
	overview := m.styles.Overview.MaxHeight(overviewH).Render(r.Overview)

	parts := []string{
		poster,
		"",
		m.styles.MetaTitle.Render(titleLine),
		m.styles.Muted.Render(rating),
		"",
		overview,
	}
	return clampRows(strings.Join(parts, "\n"), totalH)
}

func blankRect(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	line := strings.Repeat(" ", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = line
	}
	return strings.Join(rows, "\n")
}

func clampRows(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- footer

// footer always renders helpLine + a status row so the body height does
// not jitter as loading/error transitions toggle. Both rows share the
// spInline indent of the title and list items so the screen reads on a
// single left edge.
func (m model) footer() string {
	pad := lipgloss.NewStyle().Padding(0, spInline)
	helpLine := pad.Render(m.help.View(screenKeys{k: m.keys, s: m.scr}))
	status := m.statusLine()
	if status == "" {
		status = " " // reserve the row
	}
	return lipgloss.JoinVertical(lipgloss.Left, helpLine, status)
}

func (m model) statusLine() string {
	pad := lipgloss.NewStyle().Padding(0, spInline)
	switch {
	case m.loading:
		return pad.Render(m.spin.View() + " " + m.styles.Muted.Render("loading…"))
	case m.err != nil:
		return m.styles.Err.Render("error: " + m.err.Error())
	}
	return ""
}

// ---------------------------------------------------------------- helpers

func toResultItems(rs []tmdb.SearchResult) []list.Item {
	items := make([]list.Item, 0, len(rs))
	for _, r := range rs {
		items = append(items, resultItem{r: r})
	}
	return items
}
