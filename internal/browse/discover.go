package browse

import (
	"context"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stupside/castor/internal/browse/tmdb"
)

// browseMode is the source of the results list on screenBrowse. Curated is the
// five top-N tabs; Discover is the genre/sort filtered feed. A non-empty search
// query overrides both without changing the mode, so clearing the query
// restores the underlying feed.
type browseMode int

const (
	modeCurated browseMode = iota
	modeDiscover
)

// discoverState is the discover feed's own state. The filter itself (media type
// + genres) lives in the genrePicker; this holds the sort, the accumulated
// pages, and the token that invalidates in-flight pages when the filter
// changes.
type discoverState struct {
	sort        tmdb.Sort
	results     []tmdb.SearchResult
	page        int
	hasMore     bool
	loadingMore bool
	tok         int
}

type discoverDoneMsg struct {
	tok        int
	page       int
	res        []tmdb.SearchResult
	totalPages int
	err        error
}

func discoverCmd(ctx context.Context, c *tmdb.Client, tok int, p tmdb.DiscoverParams) tea.Cmd {
	return func() tea.Msg {
		pg, err := c.Discover(ctx, p)
		return discoverDoneMsg{tok: tok, page: p.Page, res: pg.Results, totalPages: pg.TotalPages, err: err}
	}
}

// discParams snapshots the current filter (from the picker) + sort as a query.
func (m model) discParams(page int) tmdb.DiscoverParams {
	return tmdb.DiscoverParams{
		MediaType: m.picker.mediaType(),
		GenreIDs:  m.picker.genreIDs(),
		Sort:      m.disc.sort,
		Page:      page,
	}
}

// enterDiscover switches to Discover mode and kicks a fresh first-page fetch,
// invalidating any in-flight discover/search via the bumped token.
func (m *model) enterDiscover() tea.Cmd {
	m.mode = modeDiscover
	if m.query.Value() != "" {
		m.query.SetValue("")
		m.queryTok++
	}
	m.disc.results = nil
	m.disc.page = 1
	m.disc.hasMore = false
	m.disc.loadingMore = false
	m.disc.tok++
	m.loading = true
	m.err = nil
	m.resize()
	return tea.Batch(discoverCmd(m.ctx, m.client, m.disc.tok, m.discParams(1)), m.spin.Tick)
}

// exitDiscover returns to the curated tabs.
func (m *model) exitDiscover() tea.Cmd {
	m.mode = modeCurated
	m.resize()
	return m.ensureTabLoaded()
}

// cycleSort advances the discover sort and refetches from page one.
func (m *model) cycleSort() tea.Cmd {
	i := slices.Index(tmdb.Sorts, m.disc.sort)
	m.disc.sort = tmdb.Sorts[(i+1)%len(tmdb.Sorts)]
	return m.enterDiscover()
}

// onDiscoverDone applies a discover page (first replaces, later pages append)
// unless a newer filter change has superseded it.
func (m *model) onDiscoverDone(msg discoverDoneMsg) tea.Cmd {
	if msg.tok != m.disc.tok {
		return nil // stale: filters changed while this was in flight
	}
	m.loading = false
	m.disc.loadingMore = false
	if msg.err != nil {
		m.err = msg.err
		return nil
	}
	m.disc.page = msg.page
	m.disc.hasMore = msg.page < msg.totalPages
	if msg.page <= 1 {
		m.disc.results = msg.res
		m.results.SetItems(toResultItems(msg.res))
		m.results.Select(0)
	} else {
		m.disc.results = append(m.disc.results, msg.res...)
		m.results.SetItems(toResultItems(m.disc.results))
	}
	return m.inspector.hover()
}

// maybeLoadMore fetches the next discover page when the cursor nears the end of
// the loaded results. It is a no-op outside Discover mode.
func (m *model) maybeLoadMore() tea.Cmd {
	if m.mode != modeDiscover || m.query.Value() != "" {
		return nil
	}
	if m.disc.loadingMore || !m.disc.hasMore {
		return nil
	}
	if m.results.Index() < len(m.disc.results)-3 {
		return nil
	}
	m.disc.loadingMore = true
	return discoverCmd(m.ctx, m.client, m.disc.tok, m.discParams(m.disc.page+1))
}
