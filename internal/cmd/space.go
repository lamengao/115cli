package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/yibing/115cli/internal/open115"
)

var spaceCmd = &cobra.Command{
	Use:   "space",
	Short: "Show account storage space",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		space, err := client.Space(context.Background())
		if err != nil {
			return err
		}
		printSpace(os.Stdout, space)
		return nil
	},
}

func printSpace(out io.Writer, space open115.Space) {
	fmt.Fprintf(out, "Remaining: %s (%d bytes)\n", formatBytes(space.Remaining), space.Remaining)
	fmt.Fprintf(out, "Total: %s (%d bytes)\n", formatBytes(space.Total), space.Total)
}
