package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/stupside/castor/internal/browse"
	"github.com/stupside/castor/internal/browse/tmdb"
)

func (a *app) castBrowseCommand() *cli.Command {
	var sourceName string

	return &cli.Command{
		Name:  "browse",
		Usage: "Browse TMDB in a TUI, then cast the selection",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "source",
				Usage:       "Source to use (must match a name in config.yaml sources)",
				Required:    true,
				Destination: &sourceName,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			src, err := a.cfg.Source(sourceName)
			if err != nil {
				return err
			}

			if a.cfg.TMDB.APIKey == "" {
				return fmt.Errorf("TMDB API key missing: set tmdb.api_key in config.yaml or CASTOR_TMDB__API_KEY env var")
			}

			sel, err := browse.Run(ctx, tmdb.New(a.cfg.TMDB.APIKey, a.cfg.Network.Timeout))
			if err != nil {
				return fmt.Errorf("browse: %w", err)
			}
			if sel.Kind == browse.KindNone {
				return nil
			}

			var urls []string
			switch sel.Kind {
			case browse.KindMovie:
				urls = src.MovieURLs(sel.TMDBID)
			case browse.KindEpisode:
				urls = src.EpisodeURLs(sel.TMDBID, sel.Season, sel.Episode)
			}

			fmt.Printf("Casting: %s\n", sel.Title)
			return a.extractAndCast(ctx, cmd, urls)
		},
	}
}
