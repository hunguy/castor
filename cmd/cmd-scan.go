package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/stupside/castor/internal/device"
)

// scanTimeout is the discovery window used when no config is available.
const scanTimeout = 5 * time.Second

// scanCommand returns the "scan" CLI subcommand. Scanning is how you discover
// the values the config needs (device names, addresses), so it must not
// itself require a valid config.
func (a *app) scanCommand() *cli.Command {
	return &cli.Command{
		Name:  "scan",
		Usage: "List all devices on the local network",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			timeout := scanTimeout
			if cfg, err := a.config(); err == nil {
				timeout = cfg.Network.Timeout
			} else {
				slog.DebugContext(ctx, "scanning without config", "error", err)
			}

			devices, err := device.Discover(ctx, timeout)
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			if len(devices) == 0 {
				fmt.Println("no devices found")
				return nil
			}

			for _, d := range devices {
				fmt.Printf("%s\t%s\t%s\n", d.Name, d.Type, d.Address)
			}
			return nil
		},
	}
}
