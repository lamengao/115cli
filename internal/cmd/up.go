package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up <local-path> <remote-path>",
	Short: "Upload a local file or directory",
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
		return client.Upload(context.Background(), args[0], args[1])
	},
}
