package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yibing/115cli/internal/open115"
)

var syncDeleteRemoteMissing bool

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
		return client.SyncWithOptions(context.Background(), args[0], args[1], open115.SyncOptions{
			DeleteRemoteMissing: syncDeleteRemoteMissing,
			Log: func(action, remotePath string) {
				fmt.Fprintf(os.Stdout, "%s %s\n", action, remotePath)
			},
		})
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncDeleteRemoteMissing, "delete-remote-missing", false, "delete remote files that are missing locally")
}
