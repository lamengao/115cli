package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync <local-dir-path> <remote-dir-path>",
	Short: "Sync missing local directory contents to 115",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		client.SetProgress(func(name string, done, total int64) {
			if total <= 0 {
				fmt.Fprintf(os.Stderr, "\ruploading %s", name)
				return
			}
			fmt.Fprintf(os.Stderr, "\ruploading %s %.1f%%", name, float64(done)*100/float64(total))
			if done >= total {
				fmt.Fprintln(os.Stderr)
			}
		})
		return client.Sync(context.Background(), args[0], args[1])
	},
}
