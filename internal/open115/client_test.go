package open115

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	entries    map[string][]Entry
	infos      map[string]Info
	infoErrs   map[string]error
	listCalls  []string
	batchCalls []string
	infoCalls  []string
	mkdirs     []string
	deletes    []string
	uploads    []string
	download   string
	headers    map[string]string
	lastHeader http.Header
	tasks      []CloudTask
}

func (f *fakeBackend) ListByID(ctx context.Context, id string) ([]Entry, error) {
	f.listCalls = append(f.listCalls, id)
	return append([]Entry(nil), f.entries[id]...), nil
}

func (f *fakeBackend) ListByIDBatched(ctx context.Context, id string, count int) ([]Entry, error) {
	f.batchCalls = append(f.batchCalls, id)
	return append([]Entry(nil), f.entries[id]...), nil
}

func (f *fakeBackend) InfoByID(ctx context.Context, id string) (Info, error) {
	f.infoCalls = append(f.infoCalls, id)
	if err := f.infoErrs[id]; err != nil {
		return Info{}, err
	}
	return f.infos[id], nil
}

func (f *fakeBackend) Mkdir(ctx context.Context, parentID, name string) (Entry, error) {
	e := Entry{ID: parentID + "/" + name, ParentID: parentID, Name: name, IsDir: true, UpdatedAt: time.Now()}
	f.entries[parentID] = append(f.entries[parentID], e)
	f.entries[e.ID] = nil
	f.mkdirs = append(f.mkdirs, parentID+":"+name)
	return e, nil
}

func (f *fakeBackend) Delete(ctx context.Context, entry Entry) error {
	f.deletes = append(f.deletes, entry.ParentID+":"+entry.ID)
	return nil
}

func (f *fakeBackend) DownloadURL(ctx context.Context, file Entry) (string, map[string]string, error) {
	if f.headers != nil {
		return f.download, f.headers, nil
	}
	return f.download, map[string]string{"X-Test": "1"}, nil
}

func (f *fakeBackend) UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error {
	f.uploads = append(f.uploads, parentID+":"+name)
	return nil
}

func (f *fakeBackend) Space(ctx context.Context) (Space, error) {
	return Space{Remaining: 3, Total: 5}, nil
}

func (f *fakeBackend) DownloadQuota(ctx context.Context) (DownloadQuota, error) {
	return DownloadQuota{Remaining: 1, Total: 2}, nil
}

func (f *fakeBackend) DownloadList(ctx context.Context, filter TaskFilter, page, pageSize int) ([]CloudTask, int, error) {
	var out []CloudTask
	for _, task := range f.tasks {
		if filter == "" || taskMatchesFilter(task, filter) {
			out = append(out, task)
		}
	}
	return out, len(out), nil
}

func (f *fakeBackend) DownloadAdd(ctx context.Context, url, parentID string) (CloudTask, error) {
	task := CloudTask{InfoHash: "hash", URL: url, FolderID: parentID, Status: TaskStatusWaiting}
	f.tasks = append(f.tasks, task)
	return task, nil
}

func (f *fakeBackend) DownloadDelete(ctx context.Context, hashes []string) error {
	return nil
}

func (f *fakeBackend) DownloadRetry(ctx context.Context, hash string) error {
	return nil
}

func (f *fakeBackend) DownloadClear(ctx context.Context, filter TaskFilter) error {
	return nil
}

func taskMatchesFilter(task CloudTask, filter TaskFilter) bool {
	switch filter {
	case TaskFilterCompleted:
		return task.Status == TaskStatusCompleted
	case TaskFilterFailed:
		return task.Status == TaskStatusFailed
	case TaskFilterRunning:
		return task.Status == TaskStatusDownloading || task.Status == TaskStatusWaiting
	default:
		return true
	}
}

func TestResolveNestedRemotePath(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0":   {{ID: "a", Name: "电影", IsDir: true}},
		"a":   {{ID: "b", Name: "a.mp4", IsDir: false, Size: 10}},
		"b":   nil,
		"bad": nil,
	}}
	client := newWithBackend("0", fb, nil)
	got, err := client.Resolve(context.Background(), "/电影/a.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "b" || got.Name != "a.mp4" {
		t.Fatalf("resolved %#v", got)
	}
}

func TestListSortsDirectoriesFirst(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {
			{ID: "f", Name: "z.txt", IsDir: false},
			{ID: "d", Name: "abc", IsDir: true},
		},
	}}
	client := newWithBackend("0", fb, nil)
	got, err := client.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "abc" || !got[0].IsDir {
		t.Fatalf("entries = %#v", got)
	}
}

func TestResolveDoesNotRequireRootInfo(t *testing.T) {
	fb := &fakeBackend{
		entries: map[string][]Entry{
			"0": {{ID: "d", Name: "test", IsDir: true}},
		},
		infoErrs: map[string]error{
			"0": errors.New("参数错误 (code 1001)"),
		},
	}
	client := newWithBackend("0", fb, nil)
	got, err := client.Resolve(context.Background(), "/test")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "d" || got.Name != "test" {
		t.Fatalf("resolved %#v", got)
	}
	if len(fb.infoCalls) != 0 {
		t.Fatalf("info calls = %#v", fb.infoCalls)
	}
	if strings.Join(fb.listCalls, ",") != "0" {
		t.Fatalf("list calls = %#v", fb.listCalls)
	}
}

func TestListUsesBatchedListingForLargeDirectories(t *testing.T) {
	fb := &fakeBackend{
		entries: map[string][]Entry{
			"0": {{ID: "d", Name: "big", IsDir: true}},
			"d": {{ID: "f", Name: "z.txt", IsDir: false}},
		},
		infos: map[string]Info{
			"d": {Files: largeDirectoryThreshold + 1},
		},
	}
	client := newWithBackend("0", fb, nil)
	got, err := client.List(context.Background(), "/big")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "z.txt" {
		t.Fatalf("entries = %#v", got)
	}
	if strings.Join(fb.infoCalls, ",") != "d" {
		t.Fatalf("info calls = %#v", fb.infoCalls)
	}
	if strings.Join(fb.batchCalls, ",") != "d" {
		t.Fatalf("batch calls = %#v", fb.batchCalls)
	}
	if strings.Join(fb.listCalls, ",") != "0" {
		t.Fatalf("regular list calls = %#v", fb.listCalls)
	}
}

func TestInfoUsesDirectDirectoryInfo(t *testing.T) {
	created := time.Date(2026, 5, 6, 2, 29, 0, 0, time.UTC)
	updated := time.Date(2026, 5, 6, 2, 30, 0, 0, time.UTC)
	fb := &fakeBackend{
		entries: map[string][]Entry{
			"0": {
				{ID: "d", Name: "testupdir", IsDir: true, CreatedAt: created, UpdatedAt: updated},
			},
		},
		infos: map[string]Info{
			"d": {
				Entry:   Entry{ID: "d", Name: "testupdir", IsDir: true, CreatedAt: created, UpdatedAt: updated},
				Size:    24,
				Folders: 1,
				Files:   4,
			},
		},
	}
	client := newWithBackend("0", fb, nil)
	got, err := client.Info(context.Background(), "/testupdir")
	if err != nil {
		t.Fatal(err)
	}
	if got.Entry.ID != "d" || got.Size != 24 || got.Folders != 1 || got.Files != 4 {
		t.Fatalf("info = %#v", got)
	}
	if !got.Entry.CreatedAt.Equal(created) || !got.Entry.UpdatedAt.Equal(updated) {
		t.Fatalf("times = %#v", got.Entry)
	}
	if strings.Join(fb.infoCalls, ",") != "d" {
		t.Fatalf("info calls = %#v", fb.infoCalls)
	}
	if strings.Join(fb.listCalls, ",") != "0" {
		t.Fatalf("list calls = %#v", fb.listCalls)
	}
}

func TestInfoRejectsFiles(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "f", Name: "a.txt", IsDir: false, Size: 10}},
	}}
	client := newWithBackend("0", fb, nil)
	if _, err := client.Info(context.Background(), "/a.txt"); err == nil {
		t.Fatal("expected error for file info")
	}
}

func TestDeleteRemotePath(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "d", ParentID: "0", Name: "dst", IsDir: true}},
		"d": {{ID: "f", ParentID: "d", Name: "old.txt", IsDir: false}},
	}}
	client := newWithBackend("0", fb, nil)
	if err := client.Delete(context.Background(), "/dst/old.txt"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.deletes, ",") != "d:f" {
		t.Fatalf("deletes = %#v", fb.deletes)
	}
}

func TestDeleteRefusesRoot(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{"0": nil}}
	client := newWithBackend("0", fb, nil)
	if err := client.Delete(context.Background(), "/"); err == nil {
		t.Fatal("expected error deleting root")
	}
	if len(fb.deletes) != 0 {
		t.Fatalf("deletes = %#v", fb.deletes)
	}
}

func TestMkdirCreatesNestedRemoteDirectory(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{"0": nil}}
	client := newWithBackend("0", fb, nil)
	if err := client.Mkdir(context.Background(), "/backup/newdir"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "0:backup,0/backup:newdir" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
}

func TestMkdirAcceptsExistingRemoteDirectory(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "r", Name: "backup", IsDir: true}},
		"r": nil,
	}}
	client := newWithBackend("0", fb, nil)
	if err := client.Mkdir(context.Background(), "/backup"); err != nil {
		t.Fatal(err)
	}
	if len(fb.mkdirs) != 0 {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
}

func TestDownloadResumesPartFile(t *testing.T) {
	var rangeHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader = r.Header.Get("Range")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("world"))
	}))
	defer server.Close()
	fb := &fakeBackend{
		entries:  map[string][]Entry{"0": {{ID: "f", Name: "hello.txt", Size: 10, PickCode: "pc"}}},
		download: server.URL,
	}
	dir := t.TempDir()
	part := filepath.Join(dir, "hello.txt.part")
	if err := os.WriteFile(part, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := newWithBackend("0", fb, server.Client())
	if err := client.Download(context.Background(), "/hello.txt", filepath.Join(dir, "hello.txt")); err != nil {
		t.Fatal(err)
	}
	if rangeHeader != "bytes=5-" {
		t.Fatalf("range header = %q", rangeHeader)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "helloworld" {
		t.Fatalf("data = %q", data)
	}
}

func TestDownloadPreservesHeadersAcrossRedirect(t *testing.T) {
	var gotCookie string
	var gotUA string
	var targetURL string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetURL, http.StatusFound)
	}))
	defer redirect.Close()
	targetURL = target.URL

	fb := &fakeBackend{
		entries:  map[string][]Entry{"0": {{ID: "f", Name: "hello.txt", Size: 2, PickCode: "pc"}}},
		download: redirect.URL,
		headers: map[string]string{
			"Cookie":     "UID=test; CID=test",
			"User-Agent": "115cli-test",
		},
	}
	dir := t.TempDir()
	client := newWithBackend("0", fb, redirect.Client())
	if err := client.Download(context.Background(), "/hello.txt", filepath.Join(dir, "hello.txt")); err != nil {
		t.Fatal(err)
	}
	if gotCookie != "UID=test; CID=test" {
		t.Fatalf("cookie header = %q", gotCookie)
	}
	if gotUA != "115cli-test" {
		t.Fatalf("user-agent header = %q", gotUA)
	}
}

func TestDownloadRemovesNewPartFileOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()
	fb := &fakeBackend{
		entries:  map[string][]Entry{"0": {{ID: "f", Name: "hello.txt", Size: 2, PickCode: "pc"}}},
		download: server.URL,
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	client := newWithBackend("0", fb, server.Client())
	if err := client.Download(context.Background(), "/hello.txt", target); err == nil {
		t.Fatal("expected download error")
	}
	if _, err := os.Stat(target + ".part"); !os.IsNotExist(err) {
		t.Fatalf("part file should be removed, stat err = %v", err)
	}
}

func TestUploadFileToExistingRemoteDir(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "d", Name: "dst", IsDir: true}},
		"d": nil,
	}}
	local := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := newWithBackend("0", fb, nil)
	if err := client.Upload(context.Background(), local, "/dst"); err != nil {
		t.Fatal(err)
	}
	if len(fb.uploads) != 1 || fb.uploads[0] != "d:a.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}

func TestUploadDirectoryCreatesMissingRemotePath(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{"0": nil}}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := newWithBackend("0", fb, nil)
	if err := client.Upload(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "0:backup,0/backup:nested" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
	if len(fb.uploads) != 1 || fb.uploads[0] != "0/backup/nested:a.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}

func TestSyncUploadsOnlyMissingChildren(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {
			{ID: "r", Name: "backup", IsDir: true},
		},
		"r": {
			{ID: "existing", ParentID: "r", Name: "existing.txt", IsDir: false, Size: 5},
			{ID: "nested", Name: "nested", IsDir: true},
		},
		"nested": {
			{ID: "kept", ParentID: "nested", Name: "kept.txt", IsDir: false, Size: 5},
		},
	}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "kept.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "missing.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "newdir", "inside.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "r:newdir" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
	if strings.Join(fb.uploads, ",") != "nested:missing.txt,r:new.txt,r/newdir:inside.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
	if len(fb.deletes) != 0 {
		t.Fatalf("deletes = %#v", fb.deletes)
	}
}

func TestSyncReplacesSameNameFileWhenSizeDiffers(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {
			{ID: "r", Name: "backup", IsDir: true},
		},
		"r": {
			{ID: "old", ParentID: "r", Name: "changed.txt", IsDir: false, Size: 3},
			{ID: "same", ParentID: "r", Name: "same.txt", IsDir: false, Size: 4},
		},
	}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "same.txt"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.deletes, ",") != "r:old" {
		t.Fatalf("deletes = %#v", fb.deletes)
	}
	if strings.Join(fb.uploads, ",") != "r:changed.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}

func TestSyncUsesBatchedListingForLargeRemoteDirectories(t *testing.T) {
	fb := &fakeBackend{
		entries: map[string][]Entry{
			"0": {
				{ID: "r", Name: "backup", IsDir: true},
			},
			"r": {
				{ID: "same", ParentID: "r", Name: "same.txt", IsDir: false, Size: 4},
			},
		},
		infos: map[string]Info{
			"0": {Folders: largeDirectoryThreshold + 1},
			"r": {Files: largeDirectoryThreshold + 1},
		},
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "same.txt"), []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.batchCalls, ",") != "r" {
		t.Fatalf("batch calls = %#v", fb.batchCalls)
	}
	if strings.Join(fb.listCalls, ",") != "0" {
		t.Fatalf("regular list calls = %#v", fb.listCalls)
	}
	if len(fb.uploads) != 0 || len(fb.deletes) != 0 {
		t.Fatalf("uploads = %#v, deletes = %#v", fb.uploads, fb.deletes)
	}
}

func TestSyncCanDeleteRemoteFilesMissingLocally(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {
			{ID: "r", Name: "backup", IsDir: true},
		},
		"r": {
			{ID: "keep", ParentID: "r", Name: "keep.txt", IsDir: false, Size: 4},
			{ID: "extra", ParentID: "r", Name: "extra.txt", IsDir: false, Size: 5},
			{ID: "extra-dir", ParentID: "r", Name: "extra-dir", IsDir: true},
			{ID: "nested", ParentID: "r", Name: "nested", IsDir: true},
		},
		"nested": {
			{ID: "nested-extra", ParentID: "nested", Name: "old.txt", IsDir: false, Size: 3},
		},
	}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	err := client.SyncWithOptions(context.Background(), root, "/backup", SyncOptions{
		DeleteRemoteMissing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.deletes, ",") != "nested:nested-extra,r:extra" {
		t.Fatalf("deletes = %#v", fb.deletes)
	}
}

func TestSyncLogsRemoteChanges(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {
			{ID: "r", Name: "backup", IsDir: true},
		},
		"r": {
			{ID: "old", ParentID: "r", Name: "changed.txt", IsDir: false, Size: 3},
			{ID: "extra", ParentID: "r", Name: "extra.txt", IsDir: false, Size: 5},
		},
	}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "changed.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "newdir", "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}

	var logs []string
	client := newWithBackend("0", fb, nil)
	err := client.SyncWithOptions(context.Background(), root, "/backup", SyncOptions{
		DeleteRemoteMissing: true,
		Log: func(action, remotePath string) {
			logs = append(logs, action+" "+remotePath)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "delete /backup/changed.txt,upload /backup/changed.txt,upload /backup/new.txt,mkdir /backup/newdir,upload /backup/newdir/inside.txt,delete /backup/extra.txt"
	if strings.Join(logs, ",") != want {
		t.Fatalf("logs = %#v", logs)
	}
}

func TestSyncFollowsSymlinkDirectoriesAndFiles(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "r", Name: "backup", IsDir: true}},
		"r": nil,
	}}
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(target, "inside.txt")
	if err := os.WriteFile(targetFile, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, filepath.Join(root, "linkfile.txt")); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "r:linkdir" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
	if strings.Join(fb.uploads, ",") != "r/linkdir:inside.txt,r:linkfile.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}

func TestSyncSkipsDirectorySymlinkLoops(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{
		"0": {{ID: "r", Name: "backup", IsDir: true}},
		"r": nil,
	}}
	root := t.TempDir()
	childDir := filepath.Join(root, "child")
	if err := os.Mkdir(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(childDir, "back")); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "r:child" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
	if strings.Join(fb.uploads, ",") != "r/child:inside.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}

func TestSyncCreatesMissingRemoteTargetWithoutLocalBasename(t *testing.T) {
	fb := &fakeBackend{entries: map[string][]Entry{"0": nil}}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := newWithBackend("0", fb, nil)
	if err := client.Sync(context.Background(), root, "/backup"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fb.mkdirs, ",") != "0:backup" {
		t.Fatalf("mkdirs = %#v", fb.mkdirs)
	}
	if len(fb.uploads) != 1 || fb.uploads[0] != "0/backup:a.txt" {
		t.Fatalf("uploads = %#v", fb.uploads)
	}
}
