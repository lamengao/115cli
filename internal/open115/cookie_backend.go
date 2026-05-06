package open115

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cookieUA     = "Mozilla/5.0 115Browser/35.0.2.3"
	apiFileList  = "https://webapi.115.com/files"
	apiDirInfo   = "https://webapi.115.com/category/get"
	apiDirAdd    = "https://webapi.115.com/files/add"
	apiDelete    = "https://webapi.115.com/rb/delete"
	apiDownload  = "https://proapi.115.com/app/chrome/downurl"
	defaultLimit = int64(200)
	maxPageLimit = int64(1150)
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
	if id == "" {
		id = "0"
	}
	limit := defaultLimit
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	var out []Entry
	var offset int64
	for {
		q := url.Values{}
		q.Set("aid", "1")
		q.Set("cid", id)
		q.Set("o", "file_name")
		q.Set("asc", "1")
		q.Set("offset", strconv.FormatInt(offset, 10))
		q.Set("show_dir", "1")
		q.Set("limit", strconv.FormatInt(limit, 10))
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
		offset = int64(resp.Offset) + limit
		if offset >= int64(resp.Count) || len(resp.Files) == 0 {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	b.setHeaders(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
