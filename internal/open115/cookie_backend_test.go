package open115

import (
	"encoding/json"
	"testing"
)

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
