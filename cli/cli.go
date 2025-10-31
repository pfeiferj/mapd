package cli

import (
	"log"
	"os"
	"context"

	"github.com/urfave/cli/v3"
)

func Handle() {
	shouldExit := true
	cmd := &cli.Command{
		Commands: []*cli.Command{
			{
				Name:  "interactive",
				Aliases: []string{"i"},
				Usage: "Send commands to an active mapd instance",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					interactive()
					return nil
				},
			},
		},
		Name:  "Mapd",
		Usage: "Start an instance of mapd",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			shouldExit = false
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}

	if shouldExit {
		os.Exit(0)
	}

}
