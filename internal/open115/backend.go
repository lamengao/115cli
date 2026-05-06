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
}

type batchedListBackend interface {
	ListByIDBatched(ctx context.Context, id string, count int) ([]Entry, error)
}
