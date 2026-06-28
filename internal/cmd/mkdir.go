package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var mkdirCmd = &cobra.Command{
	Use:   "mkdir <remote-dir-path>",
	Short: "Create a remote directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		return client.Mkdir(context.Background(), args[0])
	},
}
