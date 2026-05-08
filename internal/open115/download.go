package open115

import (
	"context"
	"fmt"
)

const downloadPageSize = 30

func (c *Client) DownloadQuota(ctx context.Context) (DownloadQuota, error) {
	return c.api.DownloadQuota(ctx)
}

func (c *Client) DownloadTasks(ctx context.Context, filter TaskFilter) ([]CloudTask, error) {
	if err := validateTaskFilter(filter, true); err != nil {
		return nil, err
	}
	var out []CloudTask
	for page := 1; ; page++ {
		tasks, total, err := c.api.DownloadList(ctx, filter, page, downloadPageSize)
		if err != nil {
			return nil, err
		}
		out = append(out, tasks...)
		if len(tasks) == 0 || len(out) >= total {
			break
		}
	}
	return out, nil
}

func (c *Client) DownloadAdd(ctx context.Context, sourceURL, destDir string) (CloudTask, error) {
	parentID := ""
	if destDir != "" {
		entry, err := c.Resolve(ctx, destDir)
		if err != nil {
			return CloudTask{}, err
		}
		if !entry.IsDir {
			return CloudTask{}, fmt.Errorf("%s is not a directory", cleanRemote(destDir))
		}
		parentID = entry.ID
	}
	return c.api.DownloadAdd(ctx, sourceURL, parentID)
}

func (c *Client) DownloadDelete(ctx context.Context, hashes []string) error {
	if len(hashes) == 0 {
		return fmt.Errorf("at least one info_hash is required")
	}
	return c.api.DownloadDelete(ctx, hashes)
}

func (c *Client) DownloadStatus(ctx context.Context, hash string) (CloudTask, error) {
	tasks, err := c.DownloadTasks(ctx, "")
	if err != nil {
		return CloudTask{}, err
	}
	for _, task := range tasks {
		if task.InfoHash == hash {
			return task, nil
		}
	}
	return CloudTask{}, fmt.Errorf("%w: cloud download task %s", ErrNotFound, hash)
}

func (c *Client) DownloadRetry(ctx context.Context, hash string) error {
	if hash == "" {
		return fmt.Errorf("info_hash is required")
	}
	return c.api.DownloadRetry(ctx, hash)
}

func (c *Client) DownloadClear(ctx context.Context, filter TaskFilter) error {
	if err := validateTaskFilter(filter, true); err != nil {
		return err
	}
	return c.api.DownloadClear(ctx, filter)
}

func validateTaskFilter(filter TaskFilter, allowEmpty bool) error {
	if filter == "" && allowEmpty {
		return nil
	}
	switch filter {
	case TaskFilterCompleted, TaskFilterFailed, TaskFilterRunning:
		return nil
	default:
		return fmt.Errorf("invalid task filter %q", filter)
	}
}
