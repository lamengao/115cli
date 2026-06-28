package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var sha1Cmd = &cobra.Command{
	Use:   "sha1 <remote-file>",
	Short: "Show remote file SHA1",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		sha1, err := client.SHA1(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, sha1)
		return nil
	},
}
