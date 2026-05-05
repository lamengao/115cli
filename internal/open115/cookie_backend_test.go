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
