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
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DownloadURL string
}

type Info struct {
	Entry   Entry
	Size    int64
	Files   int
	Folders int
}

type ProgressFunc func(name string, done, total int64)
