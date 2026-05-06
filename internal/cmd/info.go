package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/yibing/115cli/internal/open115"
)

var infoCmd = &cobra.Command{
	Use:   "info remote-dir",
	Short: "Show remote directory information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		info, err := client.Info(context.Background(), args[0])
		if err != nil {
			return err
		}
		printInfo(os.Stdout, args[0], info)
		return nil
	},
}

func printInfo(out io.Writer, remotePath string, info open115.Info) {
	entry := info.Entry
	fmt.Fprintf(out, "Name: %s\n", entry.Name)
	fmt.Fprintf(out, "Path: %s\n", cleanInfoPath(remotePath))
	fmt.Fprintf(out, "ID: %s\n", entry.ID)
	fmt.Fprintln(out, "Type: folder")
	fmt.Fprintf(out, "Size: %s (%d bytes)\n", formatBytes(info.Size), info.Size)
	fmt.Fprintf(out, "Contains: %s, %s\n", plural(info.Folders, "folder"), plural(info.Files, "file"))
	fmt.Fprintf(out, "Created: %s\n", formatInfoTime(entry.CreatedAt))
	fmt.Fprintf(out, "Updated: %s\n", formatInfoTime(entry.UpdatedAt))
}

func cleanInfoPath(remotePath string) string {
	if remotePath == "" {
		return "/"
	}
	return remotePath
}

func plural(n int, singular string) string {
	word := singular
	if n != 1 {
		word += "s"
	}
	return strconv.Itoa(n) + " " + word
}

func formatInfoTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	value := float64(size)
	for i, suffix := range units {
		value /= float64(unit)
		if value < float64(unit) || i == len(units)-1 {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%dB", size)
}
