package browse

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eliukblau/pixterm/pkg/ansimage"
)

// posterReadyMsg is delivered when a poster has been fetched and rendered
// (or has failed). PosterPath identifies which poster this is for, so the
// model can drop the result if the user has already navigated elsewhere.
type posterReadyMsg struct {
	posterPath string
	ansi       string
	err        error
}

// fetchPosterCmd downloads the poster at url and renders it to a string of
// ANSI escapes sized to (cols × rows) terminal cells. Half-block rendering
// means each cell shows 2 vertical pixels, so we ask ansimage for 2*rows
// pixels of height.
func fetchPosterCmd(ctx context.Context, url, posterPath string, cols, rows int) tea.Cmd {
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(c, http.MethodGet, url, nil)
		if err != nil {
			return posterReadyMsg{posterPath: posterPath, err: err}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return posterReadyMsg{posterPath: posterPath, err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode/100 != 2 {
			return posterReadyMsg{posterPath: posterPath, err: fmt.Errorf("poster: status %d", resp.StatusCode)}
		}

		// Half-block renders 2 vertical pixels per cell, so target pixel
		// height is rows*2. NoDithering uses true-color ▀ glyphs, which is
		// the highest-quality mode this lib offers; ScaleModeResize stretches
		// to exactly fit our reserved footprint so the layout never shifts.
		img, err := ansimage.NewScaledFromReader(
			resp.Body,
			rows*2, cols,
			color.Transparent,
			ansimage.ScaleModeResize,
			ansimage.NoDithering,
		)
		if err != nil {
			return posterReadyMsg{posterPath: posterPath, err: err}
		}
		return posterReadyMsg{posterPath: posterPath, ansi: img.Render()}
	}
}
