package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yibing/115cli/internal/open115"
)

func TestPrintInfo(t *testing.T) {
	info := open115.Info{
		Entry: open115.Entry{
			ID:        "123",
			Name:      "testupdir",
			IsDir:     true,
			CreatedAt: time.Date(2026, 5, 6, 2, 29, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 6, 2, 29, 0, 0, time.UTC),
		},
		Size:    24,
		Folders: 1,
		Files:   4,
	}
	var out bytes.Buffer
	printInfo(&out, "/testupdir", info)
	got := out.String()
	for _, want := range []string{
		"Name: testupdir",
		"Path: /testupdir",
		"ID: 123",
		"Type: folder",
		"Size: 24B (24 bytes)",
		"Contains: 1 folder, 4 files",
		"Created: 2026-05-06 02:29:00",
		"Updated: 2026-05-06 02:29:00",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[int64]string{
		24:              "24B",
		1024:            "1.0KB",
		1536:            "1.5KB",
		5 * 1024 * 1024: "5.0MB",
	}
	for size, want := range tests {
		if got := formatBytes(size); got != want {
			t.Fatalf("formatBytes(%d) = %q, want %q", size, got, want)
		}
	}
}
