package open115

import (
	"context"
	"io"
)

type backend interface {
	ListByID(ctx context.Context, id string) ([]Entry, error)
	InfoByID(ctx context.Context, id string) (Info, error)
	Mkdir(ctx context.Context, parentID, name string) (Entry, error)
	Delete(ctx context.Context, entry Entry) error
	DownloadURL(ctx context.Context, file Entry) (string, map[string]string, error)
	UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error
	Space(ctx context.Context) (Space, error)
	DownloadQuota(ctx context.Context) (DownloadQuota, error)
	DownloadList(ctx context.Context, filter TaskFilter, page, pageSize int) ([]CloudTask, int, error)
	DownloadAdd(ctx context.Context, url, parentID string) (CloudTask, error)
	DownloadDelete(ctx context.Context, hashes []string) error
	DownloadRetry(ctx context.Context, hash string) error
	DownloadClear(ctx context.Context, filter TaskFilter) error
}

type batchedListBackend interface {
	ListByIDBatched(ctx context.Context, id string, count int) ([]Entry, error)
}
