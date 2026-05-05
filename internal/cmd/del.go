package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var delCmd = &cobra.Command{
	Use:   "del <remote-path>",
	Short: "Delete a remote file or directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		return client.Delete(context.Background(), args[0])
	},
}
