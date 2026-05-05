package open115

import (
	"context"
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
	mkdirs     []string
	deletes    []string
	uploads    []string
	download   string
	lastHeader http.Header
}

func (f *fakeBackend) ListByID(ctx context.Context, id string) ([]Entry, error) {
	return append([]Entry(nil), f.entries[id]...), nil
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
	return f.download, map[string]string{"X-Test": "1"}, nil
}

func (f *fakeBackend) UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error {
	f.uploads = append(f.uploads, parentID+":"+name)
	return nil
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
