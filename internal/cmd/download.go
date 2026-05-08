package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yibing/115cli/internal/open115"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Manage cloud download tasks",
}

var downloadQuotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Show cloud download quota",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		quota, err := client.DownloadQuota(context.Background())
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Remaining: %d\n", quota.Remaining)
		fmt.Fprintf(os.Stdout, "Total: %d\n", quota.Total)
		return nil
	},
}

var downloadListFilter string

var downloadListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cloud download tasks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		tasks, err := client.DownloadTasks(context.Background(), open115.TaskFilter(downloadListFilter))
		if err != nil {
			return err
		}
		printDownloadTasks(os.Stdout, tasks)
		return nil
	},
}

var downloadAddDest string

var downloadAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a cloud download task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		task, err := client.DownloadAdd(context.Background(), args[0], downloadAddDest)
		if err != nil {
			return err
		}
		printDownloadTask(os.Stdout, task)
		return nil
	},
}

var downloadDeleteCmd = &cobra.Command{
	Use:   "delete <info_hash> [info_hash...]",
	Short: "Delete cloud download tasks",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		if err := client.DownloadDelete(context.Background(), args); err != nil {
			return err
		}
		for _, hash := range args {
			fmt.Fprintf(os.Stdout, "Deleted: %s\n", hash)
		}
		return nil
	},
}

var downloadStatusCmd = &cobra.Command{
	Use:   "status <info_hash>",
	Short: "Show a cloud download task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		task, err := client.DownloadStatus(context.Background(), args[0])
		if err != nil {
			return err
		}
		printDownloadTask(os.Stdout, task)
		return nil
	},
}

var downloadRetryCmd = &cobra.Command{
	Use:   "retry <info_hash>",
	Short: "Retry a cloud download task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		if err := client.DownloadRetry(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Retried: %s\n", args[0])
		return nil
	},
}

var downloadClearFilter string

var downloadClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear cloud download tasks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		filter := open115.TaskFilter(downloadClearFilter)
		if err := client.DownloadClear(context.Background(), filter); err != nil {
			return err
		}
		if filter == "" {
			fmt.Fprintln(os.Stdout, "Cleared completed tasks")
		} else {
			fmt.Fprintf(os.Stdout, "Cleared %s tasks\n", filter)
		}
		return nil
	},
}

func init() {
	downloadListCmd.Flags().StringVar(&downloadListFilter, "filter", "", "filter tasks by status: completed, failed, running")
	downloadAddCmd.Flags().StringVar(&downloadAddDest, "dest", "", "destination remote folder")
	downloadClearCmd.Flags().StringVar(&downloadClearFilter, "filter", "", "filter tasks to clear: completed, failed, running")
	downloadCmd.AddCommand(downloadQuotaCmd, downloadListCmd, downloadAddCmd, downloadDeleteCmd, downloadStatusCmd, downloadRetryCmd, downloadClearCmd)
}

func printDownloadTasks(out io.Writer, tasks []open115.CloudTask) {
	widths := downloadColumnWidths{
		hash:     len("HASH"),
		status:   len("STATUS"),
		progress: len("PROGRESS"),
		size:     len("SIZE"),
	}
	rows := make([]downloadRow, 0, len(tasks))
	for _, task := range tasks {
		row := downloadRow{
			hash:     task.InfoHash,
			status:   downloadStatusLabel(task.Status),
			progress: fmt.Sprintf("%.1f%%", task.PercentDone),
			size:     formatBytes(task.Size),
			name:     task.Name,
		}
		rows = append(rows, row)
		widths.hash = max(widths.hash, len(row.hash))
		widths.status = max(widths.status, len(row.status))
		widths.progress = max(widths.progress, len(row.progress))
		widths.size = max(widths.size, len(row.size))
	}
	fmt.Fprintln(out, formatDownloadRow(widths, "HASH", "STATUS", "PROGRESS", "SIZE", "NAME"))
	for _, row := range rows {
		fmt.Fprintln(out, formatDownloadRow(widths, row.hash, row.status, row.progress, row.size, row.name))
	}
}

func printDownloadTask(out io.Writer, task open115.CloudTask) {
	fmt.Fprintf(out, "Hash: %s\n", task.InfoHash)
	fmt.Fprintf(out, "Name: %s\n", emptyDash(task.Name))
	fmt.Fprintf(out, "Size: %s (%d bytes)\n", formatBytes(task.Size), task.Size)
	fmt.Fprintf(out, "Status: %s\n", downloadStatusLabel(task.Status))
	fmt.Fprintf(out, "Progress: %.1f%%\n", task.PercentDone)
	fmt.Fprintf(out, "URL: %s\n", emptyDash(task.URL))
	fmt.Fprintf(out, "File ID: %s\n", emptyDash(task.FileID))
	fmt.Fprintf(out, "Pick Code: %s\n", emptyDash(task.PickCode))
	fmt.Fprintf(out, "Folder ID: %s\n", emptyDash(task.FolderID))
	fmt.Fprintf(out, "Added: %s\n", formatInfoTime(task.AddTime))
}

type downloadColumnWidths struct {
	hash     int
	status   int
	progress int
	size     int
}

type downloadRow struct {
	hash     string
	status   string
	progress string
	size     string
	name     string
}

func formatDownloadRow(widths downloadColumnWidths, hash, status, progress, size, name string) string {
	return padRight(hash, len(hash), widths.hash) + "  " +
		padRight(status, len(status), widths.status) + "  " +
		padLeft(progress, len(progress), widths.progress) + "  " +
		padLeft(size, len(size), widths.size) + "  " +
		name
}

func downloadStatusLabel(status open115.TaskStatus) string {
	switch status {
	case open115.TaskStatusFailed:
		return "failed"
	case open115.TaskStatusWaiting:
		return "waiting"
	case open115.TaskStatusDownloading:
		return "downloading"
	case open115.TaskStatusCompleted:
		return "completed"
	default:
		return strings.TrimSpace(fmt.Sprintf("%d", status))
	}
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
