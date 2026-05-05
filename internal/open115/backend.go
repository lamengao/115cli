package open115

import (
	"context"
	"io"
)

type backend interface {
	ListByID(ctx context.Context, id string) ([]Entry, error)
	Mkdir(ctx context.Context, parentID, name string) (Entry, error)
	Delete(ctx context.Context, entry Entry) error
	DownloadURL(ctx context.Context, file Entry) (string, map[string]string, error)
	UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error
}
