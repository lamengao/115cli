package open115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("remote path not found")

type Client struct {
	rootID   string
	api      backend
	http     *http.Client
	progress ProgressFunc
}

type SyncOptions struct {
	DeleteRemoteMissing bool
}

func New(refreshToken, accessToken, rootID string, onTokenRefresh func(accessToken, refreshToken string)) *Client {
	if rootID == "" {
		rootID = "0"
	}
	return &Client{
		rootID: rootID,
		api:    newSDKBackend(refreshToken, accessToken, onTokenRefresh),
		http:   http.DefaultClient,
	}
}

func newWithBackend(rootID string, api backend, hc *http.Client) *Client {
	if rootID == "" {
		rootID = "0"
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{rootID: rootID, api: api, http: hc}
}

func (c *Client) SetProgress(progress ProgressFunc) {
	c.progress = progress
}

func (c *Client) List(ctx context.Context, remotePath string) ([]Entry, error) {
	entry, err := c.Resolve(ctx, remotePath)
	if err != nil {
		return nil, err
	}
	if !entry.IsDir {
		return nil, fmt.Errorf("%s is not a directory", cleanRemote(remotePath))
	}
	entries, err := c.api.ListByID(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (c *Client) Info(ctx context.Context, remotePath string) (Info, error) {
	entry, err := c.Resolve(ctx, remotePath)
	if err != nil {
		return Info{}, err
	}
	if !entry.IsDir {
		return Info{}, fmt.Errorf("%s is not a directory", cleanRemote(remotePath))
	}
	info, err := c.api.InfoByID(ctx, entry.ID)
	if err != nil {
		return Info{}, err
	}
	info.Entry = mergeInfoEntry(info.Entry, entry)
	return info, nil
}

func mergeInfoEntry(info, resolved Entry) Entry {
	if info.ID == "" {
		info.ID = resolved.ID
	}
	if info.ParentID == "" {
		info.ParentID = resolved.ParentID
	}
	if info.Name == "" {
		info.Name = resolved.Name
	}
	info.IsDir = resolved.IsDir
	if info.CreatedAt.IsZero() {
		info.CreatedAt = resolved.CreatedAt
	}
	if info.UpdatedAt.IsZero() {
		info.UpdatedAt = resolved.UpdatedAt
	}
	return info
}

func (c *Client) Resolve(ctx context.Context, remotePath string) (Entry, error) {
	p := cleanRemote(remotePath)
	root := Entry{ID: c.rootID, Name: "/", IsDir: true}
	if p == "/" {
		return root, nil
	}
	cur := root
	for _, part := range splitRemote(p) {
		if !cur.IsDir {
			return Entry{}, fmt.Errorf("%s is not a directory", cur.Name)
		}
		children, err := c.api.ListByID(ctx, cur.ID)
		if err != nil {
			return Entry{}, err
		}
		found := false
		for _, child := range children {
			if child.Name == part {
				cur = child
				found = true
				break
			}
		}
		if !found {
			return Entry{}, fmt.Errorf("%w: %s", ErrNotFound, p)
		}
	}
	return cur, nil
}

func (c *Client) Download(ctx context.Context, remotePath, localPath string) error {
	entry, err := c.Resolve(ctx, remotePath)
	if err != nil {
		return err
	}
	if entry.IsDir {
		target := localPath
		if target == "" {
			if entry.Name == "/" {
				target = "."
			} else {
				target = entry.Name
			}
		}
		return c.downloadDir(ctx, entry, target)
	}
	target := localPath
	if target == "" {
		target = entry.Name
	} else if st, err := os.Stat(target); err == nil && st.IsDir() {
		target = filepath.Join(target, entry.Name)
	}
	return c.downloadFile(ctx, entry, target)
}

func (c *Client) Upload(ctx context.Context, localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return c.uploadDir(ctx, localPath, remotePath)
	}
	parent, name, err := c.resolveUploadFileTarget(ctx, remotePath, filepath.Base(localPath))
	if err != nil {
		return err
	}
	return c.uploadOneFile(ctx, localPath, parent.ID, name)
}

func (c *Client) Sync(ctx context.Context, localDir, remoteDir string) error {
	return c.SyncWithOptions(ctx, localDir, remoteDir, SyncOptions{})
}

func (c *Client) SyncWithOptions(ctx context.Context, localDir, remoteDir string, opts SyncOptions) error {
	info, err := os.Stat(localDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a local directory", localDir)
	}
	target, err := c.resolveSyncTarget(ctx, remoteDir)
	if err != nil {
		return err
	}
	seen, err := syncSeenDirs(localDir)
	if err != nil {
		return err
	}
	return c.syncDir(ctx, localDir, target, seen, opts)
}

func (c *Client) Delete(ctx context.Context, remotePath string) error {
	p := cleanRemote(remotePath)
	if p == "/" {
		return fmt.Errorf("refusing to delete remote root")
	}
	entry, err := c.Resolve(ctx, p)
	if err != nil {
		return err
	}
	return c.api.Delete(ctx, entry)
}

func (c *Client) downloadDir(ctx context.Context, dir Entry, localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	children, err := c.api.ListByID(ctx, dir.ID)
	if err != nil {
		return err
	}
	for _, child := range children {
		target := filepath.Join(localDir, child.Name)
		if child.IsDir {
			if err := c.downloadDir(ctx, child, target); err != nil {
				return err
			}
			continue
		}
		if err := c.downloadFile(ctx, child, target); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) downloadFile(ctx context.Context, file Entry, localPath string) error {
	if st, err := os.Stat(localPath); err == nil && !st.IsDir() && st.Size() == file.Size {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	url, headers, err := c.api.DownloadURL(ctx, file)
	if err != nil {
		return err
	}
	partPath := localPath + ".part"
	var start int64
	if st, err := os.Stat(partPath); err == nil {
		start = st.Size()
	}
	if start > file.Size {
		start = 0
	}
	resumeStart := start
	flag := os.O_CREATE | os.O_WRONLY
	if start > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	out, err := os.OpenFile(partPath, flag, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	resp, err := c.doDownloadRequest(req)
	if err != nil {
		c.cleanupFailedDownload(partPath, resumeStart)
		return err
	}
	defer resp.Body.Close()
	if start > 0 && resp.StatusCode == http.StatusOK {
		if err := out.Truncate(0); err != nil {
			return err
		}
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			return err
		}
		start = 0
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		c.cleanupFailedDownload(partPath, resumeStart)
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		c.cleanupFailedDownload(partPath, resumeStart)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if st, err := os.Stat(partPath); err != nil {
		return err
	} else if file.Size > 0 && st.Size() != file.Size {
		c.cleanupFailedDownload(partPath, resumeStart)
		return fmt.Errorf("download incomplete for %s: got %d bytes, want %d", file.Name, st.Size(), file.Size)
	}
	return os.Rename(partPath, localPath)
}

func (c *Client) doDownloadRequest(req *http.Request) (*http.Response, error) {
	const maxRedirects = 10
	headers := req.Header.Clone()
	for redirects := 0; ; redirects++ {
		resp, err := c.downloadHTTPClient().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			return resp, nil
		}
		if redirects >= maxRedirects {
			resp.Body.Close()
			return nil, fmt.Errorf("download failed: stopped after %d redirects", maxRedirects)
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		if location == "" {
			return nil, fmt.Errorf("download failed: redirect missing Location")
		}
		nextURL, err := req.URL.Parse(location)
		if err != nil {
			return nil, err
		}
		nextReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, nextURL.String(), nil)
		if err != nil {
			return nil, err
		}
		nextReq.Header = headers.Clone()
		req = nextReq
	}
}

func (c *Client) downloadHTTPClient() *http.Client {
	hc := *c.http
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &hc
}

func (c *Client) cleanupFailedDownload(partPath string, resumeStart int64) {
	if resumeStart > 0 {
		return
	}
	_ = os.Remove(partPath)
}

func (c *Client) uploadDir(ctx context.Context, localPath, remotePath string) error {
	target, err := c.resolveUploadDirTarget(ctx, localPath, remotePath)
	if err != nil {
		return err
	}
	var failures []string
	err = filepath.WalkDir(localPath, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p, walkErr))
			return nil
		}
		if p == localPath {
			return nil
		}
		rel, err := filepath.Rel(localPath, p)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p, err))
			return nil
		}
		parent := target
		dirParts := strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/")
		if len(dirParts) > 0 && dirParts[0] != "." {
			for _, part := range dirParts {
				next, err := c.ensureChildDir(ctx, parent, part)
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s: %v", p, err))
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				parent = next
			}
		}
		if d.IsDir() {
			_, err := c.ensureChildDir(ctx, parent, d.Name())
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", p, err))
				return filepath.SkipDir
			}
			return nil
		}
		if err := c.uploadOneFile(ctx, p, parent.ID, d.Name()); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", p, err))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("upload completed with %d failure(s):\n%s", len(failures), strings.Join(failures, "\n"))
	}
	return nil
}

func (c *Client) uploadOneFile(ctx context.Context, localPath, parentID, remoteName string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	return c.api.UploadFile(ctx, parentID, remoteName, st.Size(), f, c.progress)
}

func (c *Client) resolveSyncTarget(ctx context.Context, remoteDir string) (Entry, error) {
	if remoteDir == "" {
		remoteDir = "/"
	}
	if existing, err := c.Resolve(ctx, remoteDir); err == nil {
		if !existing.IsDir {
			return Entry{}, fmt.Errorf("%s is not a remote directory", cleanRemote(remoteDir))
		}
		return existing, nil
	}
	return c.ensureRemoteDir(ctx, remoteDir)
}

func syncSeenDirs(localDir string) (map[string]bool, error) {
	realPath, err := filepath.EvalSymlinks(localDir)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(realPath)
	if err != nil {
		return nil, err
	}
	return map[string]bool{absPath: true}, nil
}

func syncRealDir(localDir string) (string, error) {
	realPath, err := filepath.EvalSymlinks(localDir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(realPath)
}

func (c *Client) syncDir(ctx context.Context, localDir string, remoteDir Entry, seen map[string]bool, opts SyncOptions) error {
	remoteChildren, err := c.api.ListByID(ctx, remoteDir.ID)
	if err != nil {
		return err
	}
	remoteByName := make(map[string]Entry, len(remoteChildren))
	for _, child := range remoteChildren {
		remoteByName[child.Name] = child
	}

	localChildren, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}
	localByName := make(map[string]bool, len(localChildren))
	for _, child := range localChildren {
		localByName[child.Name()] = true
	}
	var failures []string
	for _, child := range localChildren {
		localPath := filepath.Join(localDir, child.Name())
		info, err := os.Stat(localPath)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
			continue
		}
		isDir := info.IsDir()
		remoteChild, exists := remoteByName[child.Name()]
		if exists {
			if isDir && remoteChild.IsDir {
				leave, ok, err := syncEnterDir(localPath, seen)
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
				} else if ok {
					if err := c.syncDir(ctx, localPath, remoteChild, seen, opts); err != nil {
						failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
					}
					leave()
				}
			} else if !isDir && !remoteChild.IsDir {
				if info.Size() != remoteChild.Size {
					if err := c.api.Delete(ctx, remoteChild); err != nil {
						failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
					} else if err := c.uploadOneFile(ctx, localPath, remoteDir.ID, child.Name()); err != nil {
						failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
					}
				}
			}
			continue
		}
		if isDir {
			leave, ok, err := syncEnterDir(localPath, seen)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
				continue
			}
			if !ok {
				continue
			}
			created, err := c.ensureChildDir(ctx, remoteDir, child.Name())
			if err != nil {
				leave()
				failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
				continue
			}
			if err := c.syncDir(ctx, localPath, created, seen, opts); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
			}
			leave()
			continue
		}
		if err := c.uploadOneFile(ctx, localPath, remoteDir.ID, child.Name()); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", localPath, err))
		}
	}
	if opts.DeleteRemoteMissing {
		for _, child := range remoteChildren {
			if child.IsDir || localByName[child.Name] {
				continue
			}
			if err := c.api.Delete(ctx, child); err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: %v", cleanRemote(remoteDir.Name), child.Name, err))
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("sync completed with %d failure(s):\n%s", len(failures), strings.Join(failures, "\n"))
	}
	return nil
}

func syncEnterDir(localDir string, seen map[string]bool) (func(), bool, error) {
	realPath, err := syncRealDir(localDir)
	if err != nil {
		return nil, false, err
	}
	if seen[realPath] {
		return nil, false, nil
	}
	seen[realPath] = true
	return func() { delete(seen, realPath) }, true, nil
}

func (c *Client) resolveUploadFileTarget(ctx context.Context, remotePath, defaultName string) (Entry, string, error) {
	if remotePath == "" {
		remotePath = "/"
	}
	if existing, err := c.Resolve(ctx, remotePath); err == nil {
		if existing.IsDir {
			return existing, defaultName, nil
		}
		parentPath := path.Dir(cleanRemote(remotePath))
		parent, err := c.Resolve(ctx, parentPath)
		return parent, existing.Name, err
	}
	parentPath, name := path.Split(strings.TrimRight(cleanRemote(remotePath), "/"))
	if name == "" {
		name = defaultName
	}
	parent, err := c.Resolve(ctx, parentPath)
	return parent, name, err
}

func (c *Client) resolveUploadDirTarget(ctx context.Context, localPath, remotePath string) (Entry, error) {
	if remotePath == "" {
		remotePath = "/"
	}
	if existing, err := c.Resolve(ctx, remotePath); err == nil {
		if !existing.IsDir {
			return Entry{}, fmt.Errorf("%s is not a remote directory", cleanRemote(remotePath))
		}
		return c.ensureChildDir(ctx, existing, filepath.Base(localPath))
	}
	parentPath, name := path.Split(strings.TrimRight(cleanRemote(remotePath), "/"))
	if name == "" {
		name = filepath.Base(localPath)
	}
	parent, err := c.ensureRemoteDir(ctx, parentPath)
	if err != nil {
		return Entry{}, err
	}
	return c.ensureChildDir(ctx, parent, name)
}

func (c *Client) ensureRemoteDir(ctx context.Context, remotePath string) (Entry, error) {
	p := cleanRemote(remotePath)
	cur := Entry{ID: c.rootID, Name: "/", IsDir: true}
	if p == "/" {
		return cur, nil
	}
	for _, part := range splitRemote(p) {
		next, err := c.ensureChildDir(ctx, cur, part)
		if err != nil {
			return Entry{}, err
		}
		cur = next
	}
	return cur, nil
}

func (c *Client) ensureChildDir(ctx context.Context, parent Entry, name string) (Entry, error) {
	children, err := c.api.ListByID(ctx, parent.ID)
	if err != nil {
		return Entry{}, err
	}
	for _, child := range children {
		if child.Name == name {
			if !child.IsDir {
				return Entry{}, fmt.Errorf("remote path component %s exists and is not a directory", name)
			}
			return child, nil
		}
	}
	return c.api.Mkdir(ctx, parent.ID, name)
}

func cleanRemote(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

func splitRemote(p string) []string {
	p = strings.Trim(cleanRemote(p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
