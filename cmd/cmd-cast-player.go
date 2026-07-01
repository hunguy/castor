package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

func (a *app) castPlayerCommand() *cli.Command {
	var pageURL string

	return &cli.Command{
		Name:  "player",
		Usage: "Cast a video from a direct player URL",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "url",
				Destination: &pageURL,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return a.extractAndCast(ctx, cmd, []string{pageURL})
		},
	}
}
