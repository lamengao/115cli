package open115

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	sdk "github.com/xhofe/115-sdk-go"
)

const userAgent = "115cli/0.1"

type sdkBackend struct {
	client *sdk.Client
}

func newSDKBackend(refreshToken, accessToken string, onTokenRefresh func(accessToken, refreshToken string)) *sdkBackend {
	opts := []sdk.Option{sdk.WithRefreshToken(refreshToken)}
	if accessToken != "" {
		opts = append(opts, sdk.WithAccessToken(accessToken))
	}
	if onTokenRefresh != nil {
		opts = append(opts, sdk.WithOnRefreshToken(onTokenRefresh))
	}
	client := sdk.New(opts...)
	client.SetUserAgent(userAgent)
	return &sdkBackend{client: client}
}

func (b *sdkBackend) ListByID(ctx context.Context, id string) ([]Entry, error) {
	var out []Entry
	const pageSize int64 = 200
	var offset int64
	for {
		resp, err := b.client.GetFiles(ctx, &sdk.GetFilesReq{
			CID:     id,
			Limit:   pageSize,
			Offset:  offset,
			ShowDir: true,
			ASC:     true,
			O:       "file_name",
		})
		if err != nil {
			return nil, err
		}
		for _, f := range resp.Data {
			out = append(out, entryFromSDK(f))
		}
		if int64(len(out)) >= resp.Count || int64(len(resp.Data)) == 0 {
			break
		}
		offset += pageSize
	}
	return out, nil
}

func (b *sdkBackend) Mkdir(ctx context.Context, parentID, name string) (Entry, error) {
	resp, err := b.client.Mkdir(ctx, parentID, name)
	if err != nil {
		return Entry{}, err
	}
	now := time.Now()
	return Entry{
		ID:        resp.FileID,
		ParentID:  parentID,
		Name:      name,
		IsDir:     true,
		UpdatedAt: now,
	}, nil
}

func (b *sdkBackend) Delete(ctx context.Context, entry Entry) error {
	_, err := b.client.DelFile(ctx, &sdk.DelFileReq{
		FileIDs:  entry.ID,
		ParentID: entry.ParentID,
	})
	return err
}

func (b *sdkBackend) DownloadURL(ctx context.Context, file Entry) (string, map[string]string, error) {
	resp, err := b.client.DownURL(ctx, file.PickCode, userAgent)
	if err != nil {
		return "", nil, err
	}
	item, ok := resp[file.ID]
	if !ok || item.URL.URL == "" {
		return "", nil, fmt.Errorf("download url not returned for %s", file.Name)
	}
	return item.URL.URL, map[string]string{"User-Agent": userAgent}, nil
}

func (b *sdkBackend) UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error {
	fullSHA1, preSHA1, err := uploadHashes(r)
	if err != nil {
		return err
	}
	resp, err := b.client.UploadInit(ctx, &sdk.UploadInitReq{
		FileName: name,
		FileSize: size,
		Target:   parentID,
		FileID:   fullSHA1,
		PreID:    preSHA1,
	})
	if err != nil {
		return err
	}
	if resp.Status == 2 {
		if progress != nil {
			progress(name, size, size)
		}
		return nil
	}
	if resp.Status == 6 || resp.Status == 7 || resp.Status == 8 {
		resp, err = b.verifyAndUploadInit(ctx, parentID, name, size, fullSHA1, preSHA1, resp, r)
		if err != nil {
			return err
		}
		if resp.Status == 2 {
			if progress != nil {
				progress(name, size, size)
			}
			return nil
		}
	}
	tokenResp, err := b.client.UploadGetToken(ctx)
	if err != nil {
		return err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return ossUpload(ctx, r, name, size, tokenResp, resp, progress)
}

func (b *sdkBackend) verifyAndUploadInit(ctx context.Context, parentID, name string, size int64, fullSHA1, preSHA1 string, resp *sdk.UploadInitResp, r io.ReadSeeker) (*sdk.UploadInitResp, error) {
	parts := strings.Split(resp.SignCheck, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid sign_check: %s", resp.SignCheck)
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	h := sha1.New()
	if _, err := io.CopyN(h, r, end-start+1); err != nil {
		return nil, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return b.client.UploadInit(ctx, &sdk.UploadInitReq{
		FileName: name,
		FileSize: size,
		Target:   parentID,
		FileID:   fullSHA1,
		PreID:    preSHA1,
		SignKey:  resp.SignKey,
		SignVal:  strings.ToUpper(hex.EncodeToString(h.Sum(nil))),
	})
}

func entryFromSDK(f sdk.GetFilesResp_File) Entry {
	return Entry{
		ID:        f.Fid,
		ParentID:  f.Pid,
		Name:      f.Fn,
		IsDir:     f.Fc == "0",
		Size:      f.FS,
		Sha1:      f.Sha1,
		PickCode:  f.Pc,
		UpdatedAt: time.Unix(f.Upt, 0),
	}
}

func uploadHashes(r io.ReadSeeker) (string, string, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	full := sha1.New()
	if _, err := io.Copy(full, r); err != nil {
		return "", "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	pre := sha1.New()
	if _, err := io.Copy(pre, io.LimitReader(r, 128*1024)); err != nil {
		return "", "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	return strings.ToUpper(hex.EncodeToString(full.Sum(nil))), strings.ToUpper(hex.EncodeToString(pre.Sum(nil))), nil
}

func ossUpload(ctx context.Context, r io.Reader, name string, size int64, token *sdk.UploadGetTokenResp, init *sdk.UploadInitResp, progress ProgressFunc) error {
	client, err := oss.New(token.Endpoint, token.AccessKeyId, token.AccessKeySecret, oss.SecurityToken(token.SecurityToken))
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(init.Bucket)
	if err != nil {
		return err
	}
	if size > 20*1024*1024 {
		seeker, ok := r.(io.ReadSeeker)
		if !ok {
			return fmt.Errorf("multipart upload requires seekable reader")
		}
		return ossMultipartUpload(ctx, bucket, seeker, name, size, init, progress)
	}
	reader := &progressReader{ctx: ctx, name: name, r: r, total: size, progress: progress}
	return bucket.PutObject(init.Object, reader,
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(init.Callback.Value.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(init.Callback.Value.CallbackVar))),
	)
}

func ossMultipartUpload(ctx context.Context, bucket *oss.Bucket, r io.ReadSeeker, name string, size int64, init *sdk.UploadInitResp, progress ProgressFunc) error {
	imur, err := bucket.InitiateMultipartUpload(init.Object, oss.Sequential())
	if err != nil {
		return err
	}
	partSize := uploadPartSize(size)
	partCount := int((size + partSize - 1) / partSize)
	parts := make([]oss.UploadPart, 0, partCount)
	var uploaded int64
	for partNumber := 1; partNumber <= partCount; partNumber++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		offset := int64(partNumber-1) * partSize
		currentSize := partSize
		if offset+currentSize > size {
			currentSize = size - offset
		}
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		pr := &progressReader{
			ctx:      ctx,
			name:     name,
			r:        io.LimitReader(r, currentSize),
			done:     uploaded,
			total:    size,
			progress: progress,
		}
		part, err := bucket.UploadPart(imur, pr, currentSize, partNumber)
		if err != nil {
			_ = bucket.AbortMultipartUpload(imur)
			return err
		}
		uploaded += currentSize
		parts = append(parts, part)
	}
	_, err = bucket.CompleteMultipartUpload(imur, parts,
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(init.Callback.Value.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(init.Callback.Value.CallbackVar))),
	)
	return err
}

func uploadPartSize(size int64) int64 {
	const (
		mb = 1024 * 1024
		gb = 1024 * mb
		tb = 1024 * gb
	)
	partSize := int64(20 * mb)
	switch {
	case size > tb:
		partSize = 5 * gb
	case size > 768*gb:
		partSize = 109951163
	case size > 512*gb:
		partSize = 82463373
	case size > 384*gb:
		partSize = 54975582
	case size > 256*gb:
		partSize = 41231687
	case size > 128*gb:
		partSize = 27487791
	}
	return partSize
}

type progressReader struct {
	ctx      context.Context
	name     string
	r        io.Reader
	done     int64
	total    int64
	progress ProgressFunc
}

func (r *progressReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	r.done += int64(n)
	if r.progress != nil && n > 0 {
		r.progress(r.name, r.done, r.total)
	}
	return n, err
}
