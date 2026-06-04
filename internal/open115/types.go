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

type DownloadQuota struct {
	Remaining int
	Total     int
}

type Space struct {
	Remaining int64
	Total     int64
}

type TaskStatus int

const (
	TaskStatusFailed      TaskStatus = -1
	TaskStatusWaiting     TaskStatus = 0
	TaskStatusDownloading TaskStatus = 1
	TaskStatusCompleted   TaskStatus = 2
)

type TaskFilter string

const (
	TaskFilterCompleted TaskFilter = "completed"
	TaskFilterFailed    TaskFilter = "failed"
	TaskFilterRunning   TaskFilter = "running"
)

type CloudTask struct {
	InfoHash    string
	Name        string
	Size        int64
	Status      TaskStatus
	PercentDone float64
	URL         string
	FileID      string
	PickCode    string
	FolderID    string
	AddTime     time.Time
}

type ProgressFunc func(name string, done, total int64)
