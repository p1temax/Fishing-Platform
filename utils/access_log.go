package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	accessLogTimeLayout    = "2006/01/02 15:04:05"
	accessLogDayLayout     = "2006-01-02"
	accessLogSlowThreshold = 500 * time.Millisecond
)

// Paths that poll frequently: successful fast responses stay off the console
// (still written to the daily access log file).
var accessLogQuietExact = map[string]struct{}{
	"/api/dashboard":  {},
	"/api/dashboard/": {},
}

// AccessLog is a day-rotated HTTP access log writer.
// Files are named access-YYYY-MM-DD.log under Dir.
type AccessLog struct {
	mu   sync.Mutex
	dir  string
	day  string
	file *os.File
	path string
}

// DefaultAccessLogDir returns the directory for access logs (next to the DB when possible).
func DefaultAccessLogDir(databasePath string) string {
	dir := filepath.Dir(strings.TrimSpace(databasePath))
	if dir == "" || dir == "." {
		return "data"
	}
	return dir
}

// AccessLogPathForDay builds access-YYYY-MM-DD.log under dir.
func AccessLogPathForDay(dir, day string) string {
	day = strings.TrimSpace(day)
	if day == "" {
		day = time.Now().Format(accessLogDayLayout)
	}
	return filepath.Join(dir, "access-"+day+".log")
}

// OpenAccessLog opens (or creates) today's access log under dir and enables daily rotation.
func OpenAccessLog(dir string) (*AccessLog, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "data"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	a := &AccessLog{dir: dir}
	if err := a.rotateLocked(time.Now()); err != nil {
		return nil, err
	}
	return a, nil
}

// Path returns the currently open log file path.
func (a *AccessLog) Path() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.path
}

// Close closes the current log file.
func (a *AccessLog) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file == nil {
		return nil
	}
	err := a.file.Close()
	a.file = nil
	return err
}

func (a *AccessLog) rotateLocked(now time.Time) error {
	day := now.Format(accessLogDayLayout)
	if a.file != nil && a.day == day {
		return nil
	}
	path := AccessLogPathForDay(a.dir, day)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	if a.file != nil {
		_ = a.file.Close()
	}
	a.file = f
	a.day = day
	a.path = path
	return nil
}

func (a *AccessLog) writePlain(line string) {
	a.writePlainAt(time.Now(), line)
}

func (a *AccessLog) writePlainAt(now time.Time, line string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.rotateLocked(now); err != nil {
		return
	}
	if a.file == nil {
		return
	}
	_, _ = io.WriteString(a.file, line+"\n")
}

func isQuietAccessPath(path string) bool {
	_, ok := accessLogQuietExact[path]
	return ok
}

func shouldSkipConsole(path string, status int, latency time.Duration) bool {
	if !isQuietAccessPath(path) {
		return false
	}
	if status < 200 || status >= 300 {
		return false
	}
	if latency >= accessLogSlowThreshold {
		return false
	}
	return true
}

func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}

func accessStatusGlyph(status int) (glyph, colorCode string) {
	switch {
	case status >= 200 && status < 300:
		return "✔", ansiGreen
	case status >= 300 && status < 400:
		return "➜", ansiCyan
	case status >= 400 && status < 500:
		return "✖", ansiYellow
	default:
		return "✖", ansiRed
	}
}

func formatAccessLine(ts time.Time, status int, method, path, client string, latency time.Duration, color bool) string {
	glyph, code := accessStatusGlyph(status)
	stamp := ts.Format(accessLogTimeLayout)
	plain := fmt.Sprintf("%s  %s %d  %s %s  %s  %s",
		stamp, glyph, status, method, path, formatLatency(latency), client)
	if !color {
		return plain
	}
	return fmt.Sprintf("%s  %s  %s %s  %s  %s",
		colorize(true, ansiDim, stamp),
		colorize(true, code+ansiBold, fmt.Sprintf("%s %d", glyph, status)),
		colorize(true, ansiBold, method),
		path,
		colorize(true, ansiDim, formatLatency(latency)),
		colorize(true, ansiDim, client),
	)
}

// AccessLogMiddleware writes every request to the daily log file (no ANSI) and
// prints a compact colored line to stdout, skipping successful fast dashboard polls.
func AccessLogMiddleware(accessLog *AccessLog) gin.HandlerFunc {
	consoleColor := stdoutIsTTY()

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.Request.URL.Path
		if raw := c.Request.URL.RawQuery; raw != "" {
			path = path + "?" + raw
		}
		method := c.Request.Method
		client := c.ClientIP()
		now := time.Now()

		plainPath := c.Request.URL.Path
		plain := formatAccessLine(now, status, method, path, client, latency, false)
		if accessLog != nil {
			accessLog.writePlain(plain)
		}

		if shouldSkipConsole(plainPath, status, latency) {
			return
		}
		line := formatAccessLine(now, status, method, path, client, latency, consoleColor)
		fmt.Fprintln(os.Stdout, line)
	}
}
