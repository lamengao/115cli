package open115

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aead/ecdh"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/andreburgaud/crypt2go/ecb"
	"github.com/andreburgaud/crypt2go/padding"
	"github.com/pierrec/lz4/v4"
)

const (
	cookieUploadAppVersion = "30.5.1"
	cookieUploadUA         = "Mozilla/5.0 115disk/" + cookieUploadAppVersion
	cookieAliUA            = "aliyun-sdk-android/2.9.1"
	cookieUploadInitURL    = "https://uplb.115.com/4.0/initupload.php?k_ec=%s"
	cookieUploadInfoURL    = "https://proapi.115.com/app/uploadinfo"
	cookieOSSInfoURL       = "https://uplb.115.com/3.0/getuploadinfo.php"
	cookieUploadTarget     = "U_1_"
	cookieUploadEnd        = "000000"
	cookieUploadMD5Salt    = "Qclm8MGWUv59TnrR0XPg"
	cookieUploadCRCSalt    = "^j>WD3Kr?J2gLFjD4W2y@"
	p224BaseLen            = 28
)

var cookieUploadRemotePubKey = []byte{
	0x57, 0xA2, 0x92, 0x57, 0xCD, 0x23, 0x20, 0xE5, 0xD6, 0xD1, 0x43, 0x32, 0x2F, 0xA4,
	0xBB, 0x8A, 0x3C, 0xF9, 0xD3, 0xCC, 0x62, 0x3E, 0xF5, 0xED, 0xAC, 0x62, 0xB7, 0x67,
	0x8A, 0x89, 0xC9, 0x1A, 0x83, 0xBA, 0x80, 0x0D, 0x61, 0x29, 0xF5, 0x22, 0xD0, 0x34,
	0xC8, 0x95, 0xDD, 0x24, 0x65, 0x24, 0x3A, 0xDD, 0xC2, 0x50, 0x95, 0x3B, 0xEE, 0xBA,
}

type cookieUploadUser struct {
	UserID  int    `json:"user_id"`
	UserKey string `json:"userkey"`
}

type cookieUploadInitResp struct {
	Status     int                  `json:"status"`
	StatusCode int                  `json:"statuscode"`
	StatusMsg  string               `json:"statusmsg"`
	PickCode   string               `json:"pickcode"`
	Target     string               `json:"target"`
	Version    string               `json:"version"`
	Bucket     string               `json:"bucket"`
	Object     string               `json:"object"`
	Callback   cookieUploadCallback `json:"callback"`
	SignKey    string               `json:"sign_key"`
	SignCheck  string               `json:"sign_check"`
}

type cookieUploadCallback struct {
	Callback    string `json:"callback"`
	CallbackVar string `json:"callback_var"`
}

type cookieUploadInfo struct {
	Endpoint    string `json:"endpoint"`
	GetTokenURL string `json:"gettokenurl"`
}

type cookieOSSToken struct {
	StatusCode      string
	AccessKeySecret string
	SecurityToken   string
	Expiration      string
	AccessKeyID     string `json:"AccessKeyId"`
}

func (b *cookieBackend) UploadFile(ctx context.Context, parentID, name string, size int64, r io.ReadSeeker, progress ProgressFunc) error {
	fullSHA1, _, err := uploadHashes(r)
	if err != nil {
		return err
	}
	user, err := b.uploadUser(ctx)
	if err != nil {
		return err
	}
	init, err := b.uploadInit(ctx, user, parentID, name, size, fullSHA1, "", "")
	if err != nil {
		return err
	}
	if init.Status == 7 && init.StatusCode == 701 {
		signVal, err := uploadRangeSHA1(r, init.SignCheck)
		if err != nil {
			return err
		}
		init, err = b.uploadInit(ctx, user, parentID, name, size, fullSHA1, init.SignKey, signVal)
		if err != nil {
			return err
		}
	}
	if init.Status == 2 && init.StatusCode == 0 {
		if progress != nil {
			progress(name, size, size)
		}
		return nil
	}
	if init.Status != 1 || init.StatusCode != 0 {
		return fmt.Errorf("115 upload init failed for %s: status=%d statuscode=%d msg=%s", name, init.Status, init.StatusCode, init.StatusMsg)
	}
	token, endpoint, err := b.cookieOSSToken(ctx)
	if err != nil {
		return err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return cookieOSSUpload(ctx, endpoint, token, init, r, name, size, progress)
}

func (b *cookieBackend) uploadUser(ctx context.Context) (cookieUploadUser, error) {
	var user cookieUploadUser
	if err := b.getCookieJSON(ctx, cookieUploadInfoURL, cookieUploadUA, &user); err != nil {
		return user, err
	}
	if user.UserID == 0 || user.UserKey == "" {
		return user, fmt.Errorf("failed to get 115 upload user key; cookie may be expired")
	}
	return user, nil
}

func (b *cookieBackend) uploadInit(ctx context.Context, user cookieUploadUser, parentID, name string, size int64, fileSHA1, signKey, signVal string) (cookieUploadInitResp, error) {
	ec, err := newCookieECDHCipher()
	if err != nil {
		return cookieUploadInitResp{}, err
	}
	userID := strconv.Itoa(user.UserID)
	target := cookieUploadTarget + parentID
	hash := sha1.Sum([]byte(userID + fileSHA1 + target + "0"))
	sigHash := sha1.Sum([]byte(user.UserKey + hex.EncodeToString(hash[:]) + cookieUploadEnd))
	sig := strings.ToUpper(hex.EncodeToString(sigHash[:]))
	now := time.Now().Unix()
	userIDMD5 := md5.Sum([]byte(userID))
	tokenMD5 := md5.Sum([]byte(cookieUploadMD5Salt + fileSHA1 + strconv.FormatInt(size, 10) + signKey + signVal + userID + strconv.FormatInt(now, 10) + hex.EncodeToString(userIDMD5[:]) + cookieUploadAppVersion))
	encodedToken, err := ec.encodeToken(now)
	if err != nil {
		return cookieUploadInitResp{}, err
	}

	form := url.Values{}
	form.Set("appid", "0")
	form.Set("appversion", cookieUploadAppVersion)
	form.Set("userid", userID)
	form.Set("filename", name)
	form.Set("filesize", strconv.FormatInt(size, 10))
	form.Set("fileid", fileSHA1)
	form.Set("target", target)
	form.Set("sig", sig)
	form.Set("t", strconv.FormatInt(now, 10))
	form.Set("token", hex.EncodeToString(tokenMD5[:]))
	if signKey != "" && signVal != "" {
		form.Set("sign_key", signKey)
		form.Set("sign_val", signVal)
	}
	encrypted, err := ec.encrypt([]byte(form.Encode()))
	if err != nil {
		return cookieUploadInitResp{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(cookieUploadInitURL, encodedToken), bytes.NewReader(encrypted))
	if err != nil {
		return cookieUploadInitResp{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	b.setCookieHeaders(req, cookieUploadUA)
	resp, err := b.client.Do(req)
	if err != nil {
		return cookieUploadInitResp{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return cookieUploadInitResp{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return cookieUploadInitResp{}, fmt.Errorf("115 upload init failed: %s", resp.Status)
	}
	plain, err := ec.decrypt(body)
	if err != nil {
		plain = body
	}
	var out cookieUploadInitResp
	if err := json.Unmarshal(plain, &out); err != nil {
		return out, fmt.Errorf("decode 115 upload init response: %w: %s", err, strings.TrimSpace(string(plain)))
	}
	return out, nil
}

func (b *cookieBackend) cookieOSSToken(ctx context.Context) (cookieOSSToken, string, error) {
	var info cookieUploadInfo
	if err := b.getCookieJSON(ctx, cookieOSSInfoURL, cookieUploadUA, &info); err != nil {
		return cookieOSSToken{}, "", err
	}
	if info.Endpoint == "" || info.GetTokenURL == "" {
		return cookieOSSToken{}, "", fmt.Errorf("115 OSS upload info missing endpoint or token URL")
	}
	var token cookieOSSToken
	if err := b.getCookieJSON(ctx, info.GetTokenURL, cookieUploadUA, &token); err != nil {
		return token, "", err
	}
	if token.AccessKeyID == "" || token.AccessKeySecret == "" || token.SecurityToken == "" {
		return token, "", fmt.Errorf("115 OSS token response missing credentials")
	}
	return token, info.Endpoint, nil
}

func (b *cookieBackend) getCookieJSON(ctx context.Context, endpoint, ua string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	b.setCookieHeaders(req, ua)
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

func (b *cookieBackend) setCookieHeaders(req *http.Request, ua string) {
	req.Header.Set("Cookie", b.cookie)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://115.com")
	req.Header.Set("Referer", "https://115.com/")
}

func uploadRangeSHA1(r io.ReadSeeker, signCheck string) (string, error) {
	parts := strings.Split(signCheck, "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid sign_check: %s", signCheck)
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", err
	}
	if start < 0 || end < start {
		return "", fmt.Errorf("invalid sign_check range: %s", signCheck)
	}
	if _, err := r.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	h := sha1.New()
	if _, err := io.CopyN(h, r, end-start+1); err != nil {
		return "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil))), nil
}

func cookieOSSUpload(ctx context.Context, endpoint string, token cookieOSSToken, init cookieUploadInitResp, r io.Reader, name string, size int64, progress ProgressFunc) error {
	client, err := oss.New(endpoint, token.AccessKeyID, token.AccessKeySecret, oss.SecurityToken(token.SecurityToken), oss.UserAgent(cookieAliUA))
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(init.Bucket)
	if err != nil {
		return err
	}
	reader := &progressReader{ctx: ctx, name: name, r: r, total: size, progress: progress}
	return bucket.PutObject(init.Object, reader,
		oss.SetHeader("x-oss-security-token", token.SecurityToken),
		oss.Callback(base64.StdEncoding.EncodeToString([]byte(init.Callback.Callback))),
		oss.CallbackVar(base64.StdEncoding.EncodeToString([]byte(init.Callback.CallbackVar))),
		oss.UserAgentHeader(cookieAliUA),
	)
}

type cookieECDHCipher struct {
	key    []byte
	iv     []byte
	pubKey []byte
}

func newCookieECDHCipher() (*cookieECDHCipher, error) {
	x := big.NewInt(0).SetBytes(cookieUploadRemotePubKey[:p224BaseLen])
	y := big.NewInt(0).SetBytes(cookieUploadRemotePubKey[p224BaseLen:])
	remotePublic := ecdh.Point{X: x, Y: y}
	p224 := ecdh.Generic(elliptic.P224())
	private, public, err := p224.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	point, ok := public.(ecdh.Point)
	if !ok {
		return nil, fmt.Errorf("unexpected ECDH public key type")
	}
	buf := make([]byte, p224BaseLen)
	point.X.FillBytes(buf)
	if big.NewInt(0).And(point.Y, big.NewInt(1)).Cmp(big.NewInt(1)) == 0 {
		buf = append([]byte{p224BaseLen + 1, 0x03}, buf...)
	} else {
		buf = append([]byte{p224BaseLen + 1, 0x02}, buf...)
	}
	secret := p224.ComputeSecret(private, remotePublic)
	return &cookieECDHCipher{
		key:    secret[:aes.BlockSize],
		iv:     secret[len(secret)-aes.BlockSize:],
		pubKey: buf,
	}, nil
}

func (c *cookieECDHCipher) encrypt(plainText []byte) ([]byte, error) {
	pad := padding.NewPkcs7Padding(aes.BlockSize)
	data, err := pad.Pad(plainText)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	mode := ecb.NewECBEncrypter(block)
	xorKey := append([]byte(nil), c.iv...)
	cipherText := make([]byte, 0, len(data))
	tmp := make([]byte, 0, aes.BlockSize)
	for i, b := range data {
		tmp = append(tmp, b^xorKey[i%aes.BlockSize])
		if i%aes.BlockSize == aes.BlockSize-1 {
			mode.CryptBlocks(xorKey, tmp)
			cipherText = append(cipherText, xorKey...)
			tmp = tmp[:0]
		}
	}
	return cipherText, nil
}

func (c *cookieECDHCipher) decrypt(cipherText []byte) ([]byte, error) {
	cipherText = cipherText[:len(cipherText)-len(cipherText)%aes.BlockSize]
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	lz4Block := make([]byte, len(cipherText))
	mode := cipher.NewCBCDecrypter(block, c.iv)
	mode.CryptBlocks(lz4Block, cipherText)
	length := int(lz4Block[0]) + int(lz4Block[1])<<8
	if length+2 > len(lz4Block) {
		return nil, fmt.Errorf("invalid encrypted upload response length")
	}
	text := make([]byte, 0x2000)
	n, err := lz4.UncompressBlock(lz4Block[2:length+2], text)
	if err != nil {
		return nil, err
	}
	return text[:n], nil
}

func (c *cookieECDHCipher) encodeToken(timestamp int64) (string, error) {
	r1, err := rand.Int(rand.Reader, big.NewInt(256))
	if err != nil {
		return "", err
	}
	r2, err := rand.Int(rand.Reader, big.NewInt(256))
	if err != nil {
		return "", err
	}
	b1, b2 := byte(r1.Uint64()), byte(r2.Uint64())
	tmp := make([]byte, 0, 48)
	ts := make([]byte, 4)
	binary.BigEndian.PutUint32(ts, uint32(timestamp))
	for i := 0; i < 15; i++ {
		tmp = append(tmp, c.pubKey[i]^b1)
	}
	tmp = append(tmp, b1, 0x73^b1, b1, b1, b1)
	for i := 0; i < 4; i++ {
		tmp = append(tmp, b1^ts[3-i])
	}
	for i := 15; i < len(c.pubKey); i++ {
		tmp = append(tmp, c.pubKey[i]^b2)
	}
	tmp = append(tmp, b2, 0x01^b2, b2, b2, b2)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(append([]byte(cookieUploadCRCSalt), tmp...)))
	for i := 0; i < 4; i++ {
		tmp = append(tmp, crc[3-i])
	}
	return base64.StdEncoding.EncodeToString(tmp), nil
}
