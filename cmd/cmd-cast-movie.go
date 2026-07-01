package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

func (a *app) castMovieCommand() *cli.Command {
	var itemID string
	var sourceName string

	return &cli.Command{
		Name:  "movie",
		Usage: "Cast a movie by item ID via a source",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "source",
				Usage:       "Source to use",
				Required:    true,
				Destination: &sourceName,
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "itemID",
				Destination: &itemID,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			src, err := a.cfg.Source(sourceName)
			if err != nil {
				return err
			}

			return a.extractAndCast(ctx, cmd, src.MovieURLs(itemID))
		},
	}
}
