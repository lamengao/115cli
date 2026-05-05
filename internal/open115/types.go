package open115

import "time"

type Entry struct {
	ID          string
	ParentID    string
	Name        string
	IsDir       bool
	Size        int64
	Sha1        string
	PickCode    string
	UpdatedAt   time.Time
	DownloadURL string
}

type ProgressFunc func(name string, done, total int64)
