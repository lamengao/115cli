package open115

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCookieDownloadCookieParsesString(t *testing.T) {
	var got cookieDownloadCookie
	if err := json.Unmarshal([]byte(`"a=b; c=d"`), &got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "a=b; c=d" {
		t.Fatalf("cookie = %q", got)
	}
}

func TestCookieDownloadCookieParsesObject(t *testing.T) {
	var got cookieDownloadCookie
	if err := json.Unmarshal([]byte(`{"b":"2","a":"1"}`), &got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "a=1; b=2" {
		t.Fatalf("cookie = %q", got)
	}
}

func TestCookieDownloadCookieParsesNameValueObject(t *testing.T) {
	var got cookieDownloadCookie
	if err := json.Unmarshal([]byte(`{"name":"x","value":"y"}`), &got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "x=y" {
		t.Fatalf("cookie = %q", got)
	}
}

func TestParseInfoSize(t *testing.T) {
	tests := map[string]int64{
		"":       0,
		"61":     61,
		"61B":    61,
		"1.5KB":  1536,
		"2 MB":   2 * 1024 * 1024,
		"3.25GB": 3489660928,
	}
	for value, want := range tests {
		got, err := parseInfoSize(value)
		if err != nil {
			t.Fatalf("parseInfoSize(%q) returned error: %v", value, err)
		}
		if got != want {
			t.Fatalf("parseInfoSize(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestCookieListByIDBatchedUsesLargePages(t *testing.T) {
	var limits []string
	var offsets []string
	b := &cookieBackend{
		cookie: "UID=test; CID=test",
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			q := req.URL.Query()
			limits = append(limits, q.Get("limit"))
			offsets = append(offsets, q.Get("offset"))
			limit, err := strconv.Atoi(q.Get("limit"))
			if err != nil {
				t.Fatal(err)
			}
			files := make([]string, 0, limit)
			for i := 0; i < limit; i++ {
				files = append(files, `{"fid":"f`+strconv.Itoa(i)+`","cid":"0","n":"a.txt"}`)
			}
			body := `{"state":true,"cid":"0","count":1200,"offset":` + q.Get("offset") + `,"data":[` + strings.Join(files, ",") + `]}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})},
	}
	got, err := b.ListByIDBatched(context.Background(), "0", 1200)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1200 {
		t.Fatalf("entry count = %d", len(got))
	}
	if strings.Join(limits, ",") != "1150,50" {
		t.Fatalf("limits = %#v", limits)
	}
	if strings.Join(offsets, ",") != "0,1150" {
		t.Fatalf("offsets = %#v", offsets)
	}
}
