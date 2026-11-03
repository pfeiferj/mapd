package cli

import (
	"context"

	"github.com/urfave/cli/v3"
	"pfeifer.dev/mapd/settings"
)

func msgqQueueSizeFlag() *cli.Int64Flag {
	return &cli.Int64Flag{
		Name:    "msgq-queue-size",
		Usage:   "Override msgq reader capacity (0: auto-detect; must match openpilot)",
		Sources: cli.EnvVars("MAPD_MSGQ_QUEUE_SIZE"),
	}
}

func configureMsgqQueueSize(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	if cmd.IsSet("msgq-queue-size") {
		if err := settings.Settings.SetMsgqQueueSizeOverride(cmd.Int64("msgq-queue-size")); err != nil {
			return ctx, err
		}
	}
	return ctx, nil
}
