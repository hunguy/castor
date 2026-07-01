// Package cmd wires the castor command tree. Configuration is loaded once in
// the root's Before hook into a typed struct the subcommand closures share —
// no metadata maps, no runtime type assertions.
package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/urfave/cli/v3"

	"github.com/stupside/castor/internal/config"
	"github.com/stupside/castor/internal/version"
)

// app carries state shared by every subcommand. cfg is populated by the root
// Before hook, which urfave/cli runs before any subcommand action.
type app struct {
	cfg *config.Config
}

// Root returns the root CLI command.
func Root() *cli.Command {
	a := &app{}
	var configPath string

	return &cli.Command{
		Name:    "castor",
		Usage:   "Cast video streams to networked devices",
		Version: version.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Path to configuration file",
				Value:       "config.yaml",
				Destination: &configPath,
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			cfg, err := config.Load(configPath)
			if err != nil {
				return ctx, err
			}
			slog.InfoContext(ctx, "config loaded", "path", configPath)
			a.cfg = cfg
			return ctx, nil
		},
		Commands: []*cli.Command{
			a.castCommand(),
			a.scanCommand(),
			infoCommand(),
		},
	}
}

func infoCommand() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Print build information",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Printf("version    %s\n", version.Version)
			fmt.Printf("commit     %s\n", version.Commit)
			fmt.Printf("build time %s\n", version.BuildTime)
			return nil
		},
	}
}
