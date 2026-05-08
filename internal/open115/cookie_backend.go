package open115

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cookieUA      = "Mozilla/5.0 115Browser/35.0.2.3"
	apiFileList   = "https://webapi.115.com/files"
	apiDirInfo    = "https://webapi.115.com/category/get"
	apiDirAdd     = "https://webapi.115.com/files/add"
	apiDelete     = "https://webapi.115.com/rb/delete"
	apiDownload   = "https://proapi.115.com/app/chrome/downurl"
	apiOffline    = "https://lixian.115.com/lixian/"
	apiOfflineWeb = "https://lixian.115.com/web/lixian/"
	defaultLimit  = int64(200)
	maxPageLimit  = int64(1150)
)

type cookieBackend struct {
	cookie string
	client *http.Client
}

func NewCookie(cookie, rootID string) *Client {
	if rootID == "" {
		rootID = "0"
	}
	httpClient := http.DefaultClient
	return &Client{
		rootID: rootID,
		api:    &cookieBackend{cookie: cookie, client: httpClient},
		http:   httpClient,
	}
}

func (b *cookieBackend) ListByID(ctx context.Context, id string) ([]Entry, error) {
	return b.listByID(ctx, id, defaultLimit, -1)
}

func (b *cookieBackend) ListByIDBatched(ctx context.Context, id string, count int) ([]Entry, error) {
	return b.listByID(ctx, id, maxPageLimit, count)
}

func (b *cookieBackend) listByID(ctx context.Context, id string, pageLimit int64, expectedCount int) ([]Entry, error) {
	if id == "" {
		id = "0"
	}
	limit := pageLimit
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	var out []Entry
	var offset int64
	for {
		requestLimit := limit
		if expectedCount >= 0 {
			remaining := int64(expectedCount) - offset
			if remaining <= 0 {
				break
			}
			if remaining < requestLimit {
				requestLimit = remaining
			}
		}
		q := url.Values{}
		q.Set("aid", "1")
		q.Set("cid", id)
		q.Set("o", "file_name")
		q.Set("asc", "1")
		q.Set("offset", strconv.FormatInt(offset, 10))
		q.Set("show_dir", "1")
		q.Set("limit", strconv.FormatInt(requestLimit, 10))
		q.Set("snap", "0")
		q.Set("natsort", "0")
		q.Set("record_open_time", "1")
		q.Set("format", "json")
		q.Set("fc_mix", "0")
		var resp cookieFileListResp
		if err := b.getJSON(ctx, apiFileList+"?"+q.Encode(), &resp); err != nil {
			return nil, err
		}
		if !resp.State {
			return nil, cookieAPIError(resp.basic())
		}
		if resp.CategoryID != "" && string(resp.CategoryID) != id {
			return nil, fmt.Errorf("115 returned unexpected category id %s for %s", resp.CategoryID, id)
		}
		for _, item := range resp.Files {
			out = append(out, entryFromCookie(item))
		}
		offset = int64(resp.Offset) + int64(len(resp.Files))
		count := resp.Count
		if expectedCount >= 0 {
			count = expectedCount
		}
		if offset >= int64(count) || len(resp.Files) == 0 {
			break
		}
	}
	return out, nil
}

func (b *cookieBackend) InfoByID(ctx context.Context, id string) (Info, error) {
	if id == "" {
		id = "0"
	}
	q := url.Values{}
	q.Set("aid", "1")
	q.Set("cid", id)
	var resp cookieDirInfoResp
	if err := b.getJSON(ctx, apiDirInfo+"?"+q.Encode(), &resp); err != nil {
		return Info{}, err
	}
	if !resp.State {
		return Info{}, cookieAPIError(resp.basic())
	}
	size, err := parseInfoSize(string(resp.Size))
	if err != nil {
		return Info{}, err
	}
	files, err := parseCookieInt(resp.Count)
	if err != nil {
		return Info{}, err
	}
	folders, err := parseCookieInt(resp.FolderCount)
	if err != nil {
		return Info{}, err
	}
	return Info{
		Entry: Entry{
			ID:        id,
			Name:      resp.FileName,
			IsDir:     true,
			PickCode:  resp.PickCode,
			Sha1:      resp.Sha1,
			CreatedAt: parseCookieTime(resp.CreateTime),
			UpdatedAt: parseCookieTime(resp.UpdateTime),
		},
		Size:    size,
		Files:   files,
		Folders: folders,
	}, nil
}

func (b *cookieBackend) Mkdir(ctx context.Context, parentID, name string) (Entry, error) {
	form := url.Values{}
	form.Set("pid", parentID)
	form.Set("cname", name)
	var resp cookieMkdirResp
	if err := b.postFormJSON(ctx, apiDirAdd, form, &resp); err != nil {
		return Entry{}, err
	}
	if !resp.State {
		return Entry{}, cookieAPIError(resp.basic())
	}
	id := resp.FileID
	if id == "" {
		id = string(resp.CID)
	}
	if id == "" {
		return Entry{}, fmt.Errorf("115 mkdir response missing folder id for %s", name)
	}
	now := time.Now()
	return Entry{
		ID:        id,
		ParentID:  parentID,
		Name:      name,
		IsDir:     true,
		UpdatedAt: now,
	}, nil
}

func (b *cookieBackend) Delete(ctx context.Context, entry Entry) error {
	form := url.Values{}
	form.Set("fid", entry.ID)
	form.Set("pid", entry.ParentID)
	var resp cookieDeleteResp
	if err := b.postFormJSON(ctx, apiDelete, form, &resp); err != nil {
		return err
	}
	if !resp.State {
		return cookieAPIError(resp.basic())
	}
	return nil
}

func (b *cookieBackend) DownloadURL(ctx context.Context, file Entry) (string, map[string]string, error) {
	key := m115GenerateKey()
	body, err := json.Marshal(map[string]string{"pickcode": file.PickCode})
	if err != nil {
		return "", nil, err
	}
	form := url.Values{}
	form.Set("data", m115Encode(body, key))
	var resp cookieDownloadResp
	endpoint := apiDownload + "?t=" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := b.postFormJSON(ctx, endpoint, form, &resp); err != nil {
		return "", nil, err
	}
	if !resp.State {
		return "", nil, cookieAPIError(resp.basic())
	}
	plain, err := m115Decode(string(resp.Data), key)
	if err != nil {
		return "", nil, err
	}
	var data map[string]cookieDownloadInfo
	if err := json.Unmarshal(plain, &data); err != nil {
		return "", nil, err
	}
	for _, info := range data {
		if info.URL.URL == "" {
			continue
		}
		cookie := strings.TrimSpace(string(info.URL.AuthCookie))
		if cookie == "" {
			cookie = b.cookie
		}
		return info.URL.URL, map[string]string{
			"User-Agent": cookieUA,
			"Cookie":     cookie,
			"Accept":     "*/*",
			"Referer":    "https://115.com/",
		}, nil
	}
	return "", nil, fmt.Errorf("download url not returned for %s", file.Name)
}

func (b *cookieBackend) DownloadQuota(ctx context.Context) (DownloadQuota, error) {
	q := url.Values{}
	q.Set("ct", "lixian")
	q.Set("ac", "get_quota_info")
	var resp cookieOfflineQuotaResp
	if err := b.getJSON(ctx, apiOffline+"?"+q.Encode(), &resp); err != nil {
		return DownloadQuota{}, err
	}
	if !resp.State.OK() {
		return DownloadQuota{}, cookieAPIError(resp.basic())
	}
	return DownloadQuota{Remaining: int(resp.Quota), Total: int(resp.Total)}, nil
}

func (b *cookieBackend) DownloadList(ctx context.Context, filter TaskFilter, page, pageSize int) ([]CloudTask, int, error) {
	q := url.Values{}
	q.Set("ct", "lixian")
	q.Set("ac", "task_lists")
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))
	if stat, ok := taskFilterStat(filter); ok {
		q.Set("stat", strconv.Itoa(stat))
	}
	var resp cookieOfflineListResp
	if err := b.getJSON(ctx, apiOffline+"?"+q.Encode(), &resp); err != nil {
		return nil, 0, err
	}
	if !resp.State.OK() {
		return nil, 0, cookieAPIError(resp.basic())
	}
	tasks := make([]CloudTask, 0, len(resp.Tasks))
	for _, task := range resp.Tasks {
		tasks = append(tasks, cloudTaskFromCookie(task))
	}
	return tasks, resp.Count, nil
}

func (b *cookieBackend) DownloadAdd(ctx context.Context, sourceURL, parentID string) (CloudTask, error) {
	form := url.Values{}
	form.Set("ct", "lixian")
	form.Set("ac", "add_task_url")
	form.Set("url", sourceURL)
	if parentID != "" {
		form.Set("wp_path_id", parentID)
	}
	var resp cookieOfflineAddResp
	if err := b.postFormJSON(ctx, apiOfflineWeb, form, &resp); err != nil {
		return CloudTask{}, err
	}
	if !resp.State.OK() {
		return CloudTask{}, cookieAPIError(resp.basic())
	}
	hash := resp.InfoHash
	if hash == "" {
		hash = resp.Data.InfoHash
	}
	if hash == "" {
		return CloudTask{}, fmt.Errorf("115 add task response missing info_hash")
	}
	return b.findDownloadTask(ctx, hash)
}

func (b *cookieBackend) DownloadDelete(ctx context.Context, hashes []string) error {
	form := url.Values{}
	form.Set("ct", "lixian")
	form.Set("ac", "task_del")
	for i, hash := range hashes {
		form.Set(fmt.Sprintf("hash[%d]", i), hash)
	}
	var resp cookieOfflineBasicResp
	if err := b.postFormJSON(ctx, apiOffline, form, &resp); err != nil {
		return err
	}
	if !resp.State.OK() {
		return cookieAPIError(resp.basic())
	}
	return nil
}

func (b *cookieBackend) DownloadRetry(ctx context.Context, hash string) error {
	task, err := b.findDownloadTaskByHash(ctx, hash)
	if err != nil {
		return err
	}
	if task.URL == "" {
		return fmt.Errorf("cloud download task %s cannot be retried: source url not returned by 115", hash)
	}
	if err := b.DownloadDelete(ctx, []string{hash}); err != nil {
		return err
	}
	_, err = b.DownloadAdd(ctx, task.URL, task.FolderID)
	return err
}

func (b *cookieBackend) DownloadClear(ctx context.Context, filter TaskFilter) error {
	flag := 0
	if filter != "" {
		flag = map[TaskFilter]int{
			TaskFilterCompleted: 0,
			TaskFilterFailed:    2,
			TaskFilterRunning:   3,
		}[filter]
	}
	form := url.Values{}
	form.Set("ct", "lixian")
	form.Set("ac", "task_clear")
	form.Set("flag", strconv.Itoa(flag))
	var resp cookieOfflineBasicResp
	if err := b.postFormJSON(ctx, apiOffline, form, &resp); err != nil {
		return err
	}
	if !resp.State.OK() {
		return cookieAPIError(resp.basic())
	}
	return nil
}

func (b *cookieBackend) findDownloadTask(ctx context.Context, hash string) (CloudTask, error) {
	task, err := b.findDownloadTaskByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return CloudTask{InfoHash: hash, Status: TaskStatusWaiting}, nil
		}
		return CloudTask{}, err
	}
	return task, nil
}

func (b *cookieBackend) findDownloadTaskByHash(ctx context.Context, hash string) (CloudTask, error) {
	for page := 1; ; page++ {
		tasks, total, err := b.DownloadList(ctx, "", page, downloadPageSize)
		if err != nil {
			return CloudTask{}, err
		}
		for _, task := range tasks {
			if task.InfoHash == hash {
				return task, nil
			}
		}
		if len(tasks) == 0 || page*downloadPageSize >= total {
			break
		}
	}
	return CloudTask{}, fmt.Errorf("%w: cloud download task %s", ErrNotFound, hash)
}

func (b *cookieBackend) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	b.setHeaders(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("115 request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *cookieBackend) postFormJSON(ctx context.Context, endpoint string, form url.Values, out any) error {
	body, err := b.postForm(ctx, endpoint, form)
	if err != nil {
		return err
	}
	return decodeCookieJSON(endpoint, body, out)
}

func (b *cookieBackend) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("115 request failed: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func decodeCookieJSON(endpoint string, body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode 115 response from %s: %w: %q", endpoint, err, responseExcerpt(body))
	}
	return nil
}

func responseExcerpt(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func (b *cookieBackend) setHeaders(req *http.Request) {
	req.Header.Set("Cookie", b.cookie)
	req.Header.Set("User-Agent", cookieUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://115.com")
	req.Header.Set("Referer", "https://115.com/")
}

func cookieAPIError(resp cookieBasicResp) error {
	msg := strings.TrimSpace(resp.Error)
	if msg == "" {
		msg = strings.TrimSpace(resp.Msg)
	}
	if msg == "" {
		msg = "115 request failed"
	}
	code := resp.ErrNo
	if code == 0 {
		code = int(resp.Errno)
	}
	return fmt.Errorf("%s (code %d)", msg, code)
}

func entryFromCookie(f cookieFileInfo) Entry {
	isDir := f.FileID == ""
	id := f.FileID
	parentID := string(f.CategoryID)
	if isDir {
		id = string(f.CategoryID)
		parentID = f.ParentID
	}
	return Entry{
		ID:        id,
		ParentID:  parentID,
		Name:      f.Name,
		IsDir:     isDir,
		Size:      int64(f.Size),
		Sha1:      f.Sha1,
		PickCode:  f.PickCode,
		CreatedAt: parseCookieTime(f.CreateTime),
		UpdatedAt: parseCookieTime(f.UpdateTime),
	}
}

func taskFilterStat(filter TaskFilter) (int, bool) {
	switch filter {
	case TaskFilterCompleted:
		return 11, true
	case TaskFilterFailed:
		return 9, true
	case TaskFilterRunning:
		return 12, true
	default:
		return 0, false
	}
}

func cloudTaskFromCookie(task cookieOfflineTask) CloudTask {
	return CloudTask{
		InfoHash:    task.InfoHash,
		Name:        task.Name,
		Size:        int64(task.Size),
		Status:      TaskStatus(task.Status),
		PercentDone: float64(task.PercentDone),
		URL:         task.URL,
		FileID:      task.FileID,
		PickCode:    task.PickCode,
		FolderID:    task.FolderID,
		AddTime:     parseCookieTime(string(task.AddTime)),
	}
}

func parseCookieInt(value cookieIntString) (int, error) {
	n, err := parseCookieInt64(value)
	return int(n), err
}

func parseCookieInt64(value cookieIntString) (int64, error) {
	s := strings.TrimSpace(string(value))
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseCookieTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(ts, 0)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("UTC+8", 8*3600)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", value, loc); err == nil {
		return t
	}
	return time.Time{}
}

type cookieIntString string

func (v *cookieIntString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = cookieIntString(s)
		return nil
	}
	var i int64
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	*v = cookieIntString(strconv.FormatInt(i, 10))
	return nil
}

type cookieStringInt int64

func (v *cookieStringInt) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		i, _ := strconv.ParseInt(s, 10, 64)
		*v = cookieStringInt(i)
		return nil
	}
	var i int64
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	*v = cookieStringInt(i)
	return nil
}

type cookieBool bool

func (v *cookieBool) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = cookieBool(s == "1" || strings.EqualFold(s, "true"))
		return nil
	}
	var ok bool
	if err := json.Unmarshal(b, &ok); err == nil {
		*v = cookieBool(ok)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = cookieBool(n != 0)
	return nil
}

func (v cookieBool) OK() bool {
	return bool(v)
}

type cookieBasicResp struct {
	Errno cookieStringInt `json:"errno,omitempty"`
	ErrNo int             `json:"errNo,omitempty"`
	Error string          `json:"error,omitempty"`
	State bool            `json:"state,omitempty"`
	Msg   string          `json:"msg,omitempty"`
}

type cookieFileListResp struct {
	Errno      cookieStringInt  `json:"errno,omitempty"`
	ErrNo      int              `json:"errNo,omitempty"`
	Error      string           `json:"error,omitempty"`
	State      bool             `json:"state,omitempty"`
	Msg        string           `json:"msg,omitempty"`
	CategoryID cookieIntString  `json:"cid"`
	Count      int              `json:"count"`
	Offset     int              `json:"offset"`
	Files      []cookieFileInfo `json:"data"`
}

func (r cookieFileListResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State, Msg: r.Msg}
}

type cookieFileInfo struct {
	CategoryID cookieIntString `json:"cid"`
	FileID     string          `json:"fid"`
	ParentID   string          `json:"pid"`
	Name       string          `json:"n"`
	Size       cookieStringInt `json:"s"`
	Sha1       string          `json:"sha"`
	PickCode   string          `json:"pc"`
	CreateTime string          `json:"tp"`
	UpdateTime string          `json:"t"`
}

type cookieDirInfoResp struct {
	Errno       cookieStringInt `json:"errno,omitempty"`
	ErrNo       int             `json:"errNo,omitempty"`
	Error       string          `json:"error,omitempty"`
	State       bool            `json:"state,omitempty"`
	Msg         string          `json:"msg,omitempty"`
	Count       cookieIntString `json:"count"`
	Size        cookieIntString `json:"size"`
	FolderCount cookieIntString `json:"folder_count"`
	FileName    string          `json:"file_name"`
	PickCode    string          `json:"pick_code"`
	Sha1        string          `json:"sha1"`
	CreateTime  string          `json:"ptime"`
	UpdateTime  string          `json:"utime"`
}

func (r cookieDirInfoResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State, Msg: r.Msg}
}

type cookieMkdirResp struct {
	Errno    cookieStringInt `json:"errno,omitempty"`
	ErrNo    int             `json:"errNo,omitempty"`
	Error    string          `json:"error,omitempty"`
	State    bool            `json:"state,omitempty"`
	Msg      string          `json:"msg,omitempty"`
	CID      cookieIntString `json:"cid"`
	FileID   string          `json:"file_id"`
	FileName string          `json:"file_name"`
}

func (r cookieMkdirResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State, Msg: r.Msg}
}

type cookieDeleteResp struct {
	Errno cookieStringInt `json:"errno,omitempty"`
	ErrNo int             `json:"errNo,omitempty"`
	Error string          `json:"error,omitempty"`
	State bool            `json:"state,omitempty"`
	Msg   string          `json:"msg,omitempty"`
}

func (r cookieDeleteResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State, Msg: r.Msg}
}

type cookieDownloadResp struct {
	Errno cookieStringInt `json:"errno,omitempty"`
	ErrNo int             `json:"errNo,omitempty"`
	Error string          `json:"error,omitempty"`
	State bool            `json:"state,omitempty"`
	Msg   string          `json:"msg,omitempty"`
	Data  string          `json:"data"`
}

func (r cookieDownloadResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State, Msg: r.Msg}
}

type cookieDownloadInfo struct {
	FileName string `json:"file_name"`
	PickCode string `json:"pick_code"`
	URL      struct {
		URL        string               `json:"url"`
		AuthCookie cookieDownloadCookie `json:"auth_cookie"`
	} `json:"url"`
}

type cookieOfflineBasicResp struct {
	Errno cookieStringInt `json:"errno,omitempty"`
	ErrNo int             `json:"errNo,omitempty"`
	Error string          `json:"error,omitempty"`
	State cookieBool      `json:"state,omitempty"`
	Msg   string          `json:"msg,omitempty"`
}

func (r cookieOfflineBasicResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State.OK(), Msg: r.Msg}
}

type cookieOfflineQuotaResp struct {
	Errno cookieStringInt `json:"errno,omitempty"`
	ErrNo int             `json:"errNo,omitempty"`
	Error string          `json:"error,omitempty"`
	State cookieBool      `json:"state,omitempty"`
	Msg   string          `json:"msg,omitempty"`
	Quota cookieStringInt `json:"quota"`
	Total cookieStringInt `json:"total"`
}

func (r cookieOfflineQuotaResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State.OK(), Msg: r.Msg}
}

type cookieOfflineListResp struct {
	Errno   cookieStringInt     `json:"errno,omitempty"`
	ErrNo   int                 `json:"errNo,omitempty"`
	Error   string              `json:"error,omitempty"`
	State   cookieBool          `json:"state,omitempty"`
	Msg     string              `json:"msg,omitempty"`
	Count   int                 `json:"count"`
	Page    int                 `json:"page"`
	PageRow int                 `json:"page_row"`
	Tasks   []cookieOfflineTask `json:"tasks"`
}

func (r cookieOfflineListResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State.OK(), Msg: r.Msg}
}

type cookieOfflineTask struct {
	InfoHash    string          `json:"info_hash"`
	Name        string          `json:"name"`
	Size        cookieStringInt `json:"size"`
	Status      cookieStringInt `json:"status"`
	PercentDone cookieFloat     `json:"percentDone"`
	URL         string          `json:"url"`
	FileID      string          `json:"file_id"`
	PickCode    string          `json:"pick_code"`
	FolderID    string          `json:"wp_path_id"`
	AddTime     cookieIntString `json:"add_time"`
}

type cookieOfflineAddResp struct {
	Errno    cookieStringInt `json:"errno,omitempty"`
	ErrNo    int             `json:"errNo,omitempty"`
	Error    string          `json:"error,omitempty"`
	State    cookieBool      `json:"state,omitempty"`
	Msg      string          `json:"msg,omitempty"`
	InfoHash string          `json:"info_hash,omitempty"`
	Data     struct {
		InfoHash string `json:"info_hash"`
	} `json:"data"`
}

func (r cookieOfflineAddResp) basic() cookieBasicResp {
	return cookieBasicResp{Errno: r.Errno, ErrNo: r.ErrNo, Error: r.Error, State: r.State.OK(), Msg: r.Msg}
}

type cookieFloat float64

func (v *cookieFloat) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		*v = cookieFloat(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*v = cookieFloat(f)
	return nil
}

type cookieDownloadCookie string

func (c *cookieDownloadCookie) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = cookieDownloadCookie(s)
		return nil
	}
	var values map[string]string
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	if name, ok := values["name"]; ok {
		if value := values["value"]; value != "" {
			*c = cookieDownloadCookie(name + "=" + value)
			return nil
		}
	}
	parts := make([]string, 0, len(values))
	for name, value := range values {
		if name == "" || value == "" {
			continue
		}
		parts = append(parts, name+"="+value)
	}
	sort.Strings(parts)
	*c = cookieDownloadCookie(strings.Join(parts, "; "))
	return nil
}
