package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <remote-path> [local-path]",
	Short: "Download a remote file or directory",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		localPath := ""
		if len(args) == 2 {
			localPath = args[1]
		}
		client, err := newClient()
		if err != nil {
			return err
		}
		return client.Download(context.Background(), args[0], localPath)
	},
}
