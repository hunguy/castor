// Package tmdb provides a minimal read-only client for The Movie Database v3 API.
// Only the endpoints needed by the browse subcommand are implemented.
package tmdb

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBase = "https://api.themoviedb.org/3"
	imageBase   = "https://image.tmdb.org/t/p/"
)

// Client is a TMDB v3 API client.
type Client struct {
	apiKey string
	base   string
	http   *http.Client
}

// SearchResult is one row of /search/multi. Movie / TV use different title
// and date fields, so both pairs are exposed and the caller picks based on
// MediaType.
type SearchResult struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"` // "movie" | "tv" | "person"
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	VoteAverage  float64 `json:"vote_average"`
	PosterPath   string  `json:"poster_path"`
}

// PosterURL returns a full URL for the poster at the given TMDB size
// (w92, w154, w185, w342, w500, original). Empty string if no poster.
func (r SearchResult) PosterURL(size string) string {
	if r.PosterPath == "" {
		return ""
	}
	return imageBase + size + r.PosterPath
}

// DisplayTitle returns the user-facing title regardless of MediaType.
func (r SearchResult) DisplayTitle() string {
	if r.Title != "" {
		return r.Title
	}
	return r.Name
}

// Year returns the 4-digit release/air year, or "" if unavailable.
func (r SearchResult) Year() string {
	d := cmp.Or(r.ReleaseDate, r.FirstAirDate)
	if len(d) >= 4 {
		return d[:4]
	}
	return ""
}

// TVDetails is /tv/{id}.
type TVDetails struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Seasons []Season `json:"seasons"`
}

// Season is a season summary from TVDetails.
type Season struct {
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date"`
}

// Episode is one entry of /tv/{id}/season/{n}.
type Episode struct {
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
}

// SeasonDetails is /tv/{id}/season/{n}.
type SeasonDetails struct {
	Episodes []Episode `json:"episodes"`
}

// New builds a Client. timeout==0 falls back to 10s.
func New(apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		apiKey: apiKey,
		base:   defaultBase,
		http:   &http.Client{Timeout: timeout},
	}
}

// SearchMulti searches across movies, TV shows, and people. People are
// filtered out here since they are never castable.
func (c *Client) SearchMulti(ctx context.Context, query string) ([]SearchResult, error) {
	var resp struct {
		Results []SearchResult `json:"results"`
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("include_adult", "false")
	if err := c.get(ctx, "/search/multi", q, &resp); err != nil {
		return nil, err
	}
	out := resp.Results[:0]
	for _, r := range resp.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			out = append(out, r)
		}
	}
	return out, nil
}

// TV fetches /tv/{id}.
func (c *Client) TV(ctx context.Context, id int) (*TVDetails, error) {
	var d TVDetails
	if err := c.get(ctx, "/tv/"+strconv.Itoa(id), nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Trending returns the week's trending movies and TV shows mixed.
func (c *Client) Trending(ctx context.Context) ([]SearchResult, error) {
	return c.list(ctx, "/trending/all/week", "")
}

// PopularMovies returns /movie/popular. MediaType is filled in as "movie".
func (c *Client) PopularMovies(ctx context.Context) ([]SearchResult, error) {
	return c.list(ctx, "/movie/popular", "movie")
}

// PopularTV returns /tv/popular. MediaType is filled in as "tv".
func (c *Client) PopularTV(ctx context.Context) ([]SearchResult, error) {
	return c.list(ctx, "/tv/popular", "tv")
}

// TopRatedMovies returns /movie/top_rated.
func (c *Client) TopRatedMovies(ctx context.Context) ([]SearchResult, error) {
	return c.list(ctx, "/movie/top_rated", "movie")
}

// TopRatedTV returns /tv/top_rated.
func (c *Client) TopRatedTV(ctx context.Context) ([]SearchResult, error) {
	return c.list(ctx, "/tv/top_rated", "tv")
}

// list is the shared "paged results" GET. forceType is "" for /trending
// (whose response already includes media_type) and "movie"/"tv" otherwise.
func (c *Client) list(ctx context.Context, path, forceType string) ([]SearchResult, error) {
	var resp struct {
		Results []SearchResult `json:"results"`
	}
	if err := c.get(ctx, path, nil, &resp); err != nil {
		return nil, err
	}
	if forceType != "" {
		for i := range resp.Results {
			resp.Results[i].MediaType = forceType
		}
	}
	return resp.Results, nil
}

// Season fetches /tv/{tvID}/season/{seasonNumber}.
func (c *Client) Season(ctx context.Context, tvID, seasonNumber int) (*SeasonDetails, error) {
	var d SeasonDetails
	path := "/tv/" + strconv.Itoa(tvID) + "/season/" + strconv.Itoa(seasonNumber)
	if err := c.get(ctx, path, nil, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Client) get(ctx context.Context, path string, extra url.Values, out any) error {
	if extra == nil {
		extra = url.Values{}
	}
	extra.Set("api_key", c.apiKey)
	u := c.base + path + "?" + extra.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("tmdb: %s: status %d", path, resp.StatusCode)
	}

	// Detect gzip by content rather than by the Content-Encoding header —
	// TMDB / its CDN sometimes returns a gzip body without a matching
	// header, which defeats net/http's transparent decompression. Peeking
	// the first 2 bytes for the gzip magic (\x1f\x8b) is reliable and
	// doesn't consume them.
	br := bufio.NewReader(resp.Body)
	var body io.Reader = br
	if peek, _ := br.Peek(2); len(peek) == 2 && peek[0] == 0x1f && peek[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("tmdb: gunzip %s: %w", path, err)
		}
		defer gz.Close()
		body = gz
	}

	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("tmdb: decode %s: %w", path, err)
	}
	return nil
}
