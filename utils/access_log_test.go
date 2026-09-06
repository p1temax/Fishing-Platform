package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShouldSkipConsoleDashboardOK(t *testing.T) {
	if !shouldSkipConsole("/api/dashboard/", 200, 200*time.Millisecond) {
		t.Fatal("expected dashboard 200 fast to be skipped on console")
	}
	if shouldSkipConsole("/api/dashboard/", 401, 1*time.Millisecond) {
		t.Fatal("401 must not be skipped")
	}
	if shouldSkipConsole("/api/dashboard/", 200, 800*time.Millisecond) {
		t.Fatal("slow 200 must not be skipped")
	}
	if shouldSkipConsole("/api/projects/", 200, 10*time.Millisecond) {
		t.Fatal("non-quiet path must not be skipped")
	}
}

func TestFormatAccessLineKeepsDate(t *testing.T) {
	ts := time.Date(2026, 9, 6, 1, 2, 3, 0, time.Local)
	line := formatAccessLine(ts, 200, "GET", "/api/dashboard", "127.0.0.1", 235*time.Millisecond, false)
	for _, part := range []string{"2026/09/06 01:02:03", "200", "GET", "/api/dashboard", "235ms", "127.0.0.1"} {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}

func TestAccessLogDailyPath(t *testing.T) {
	dir := t.TempDir()
	day := "2026-09-06"
	path := AccessLogPathForDay(dir, day)
	if filepath.Base(path) != "access-2026-09-06.log" {
		t.Fatalf("path=%q", path)
	}

	log, err := OpenAccessLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.writePlain("line-one")
	today := time.Now().Format(accessLogDayLayout)
	want := filepath.Join(dir, "access-"+today+".log")
	if log.Path() != want {
		t.Fatalf("open path=%q want %q", log.Path(), want)
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "line-one") {
		t.Fatalf("content=%q", raw)
	}

	// Force rotate to another day.
	future := time.Date(2099, 1, 2, 12, 0, 0, 0, time.Local)
	log.writePlainAt(future, "line-two")
	next := filepath.Join(dir, "access-2099-01-02.log")
	if log.Path() != next {
		t.Fatalf("rotated path=%q want %q", log.Path(), next)
	}
	raw2, err := os.ReadFile(next)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw2), "line-two") {
		t.Fatalf("rotated content=%q", raw2)
	}
}

func TestDefaultAccessLogDir(t *testing.T) {
	got := DefaultAccessLogDir("./data/fishing.db")
	if got != "data" && !strings.HasSuffix(got, "data") {
		t.Fatalf("unexpected dir %q", got)
	}
}
