package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	dirColor   = "\033[1;34m"
	resetColor = "\033[0m"
)

var lsCmd = &cobra.Command{
	Use:   "ls [remote-path]",
	Short: "List a remote directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remotePath := "/"
		if len(args) == 1 {
			remotePath = args[0]
		}
		client, err := newClient()
		if err != nil {
			return err
		}
		entries, err := client.List(context.Background(), remotePath)
		if err != nil {
			return err
		}
		rows := make([]lsRow, 0, len(entries))
		widths := lsColumnWidths{
			typ:     len("TYPE"),
			size:    len("SIZE"),
			updated: len("UPDATED"),
		}

		for _, e := range entries {
			typ := "file"
			size := fmt.Sprintf("%d", e.Size)
			name := e.Name
			if e.IsDir {
				typ = "dir"
				size = "-"
			}
			updated := "-"
			if !e.UpdatedAt.IsZero() {
				updated = e.UpdatedAt.Format("2006-01-02 15:04:05")
			}
			rows = append(rows, lsRow{
				typ:     typ,
				size:    size,
				updated: updated,
				name:    name,
				isDir:   e.IsDir,
			})
			widths.typ = max(widths.typ, len(typ))
			widths.size = max(widths.size, len(size))
			widths.updated = max(widths.updated, len(updated))
		}

		fmt.Fprintln(os.Stdout, formatLSRow(widths, "TYPE", "SIZE", "UPDATED", "NAME", false))
		for _, row := range rows {
			fmt.Fprintln(os.Stdout, formatLSRow(widths, row.typ, row.size, row.updated, row.name, row.isDir))
		}
		return nil
	},
}

type lsColumnWidths struct {
	typ     int
	size    int
	updated int
}

type lsRow struct {
	typ     string
	size    string
	updated string
	name    string
	isDir   bool
}

func formatLSRow(widths lsColumnWidths, typ, size, updated, name string, isDir bool) string {
	displayType := typ
	displayName := name
	if isDir {
		displayType = colorDir(typ)
		displayName = colorDir(name)
	}
	return padRight(displayType, len(typ), widths.typ) + "  " +
		padLeft(size, len(size), widths.size) + "  " +
		padRight(updated, len(updated), widths.updated) + "  " +
		displayName
}

func colorDir(s string) string {
	return dirColor + s + resetColor
}

func padRight(s string, visibleLen, width int) string {
	if visibleLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleLen)
}

func padLeft(s string, visibleLen, width int) string {
	if visibleLen >= width {
		return s
	}
	return strings.Repeat(" ", width-visibleLen) + s
}
