package utils

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LocalFlaskConfig struct {
	AppDir         string
	AppPath        string
	LogPath        string
	Port           uint
	BackendBaseURL string
	UseHTTPS       bool
	SSLCertPath    string
	SSLKeyPath     string
}

type LocalFlaskProcess struct {
	PID        int
	PythonPath string
}

func StartLocalFlaskApp(cfg LocalFlaskConfig) (*LocalFlaskProcess, error) {
	if cfg.AppDir == "" {
		return nil, fmt.Errorf("local Flask app directory is required")
	}
	appDir, err := filepath.Abs(cfg.AppDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local Flask app directory: %w", err)
	}
	cfg.AppDir = appDir
	if cfg.Port == 0 {
		cfg.Port = 5000
	}
	if cfg.AppPath == "" {
		cfg.AppPath = filepath.Join(cfg.AppDir, "app.py")
	} else if !filepath.IsAbs(cfg.AppPath) {
		absAppPath, err := filepath.Abs(cfg.AppPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve local Flask app path: %w", err)
		}
		cfg.AppPath = absAppPath
	}
	if cfg.LogPath == "" {
		cfg.LogPath = filepath.Join(cfg.AppDir, "flask.log")
	} else if !filepath.IsAbs(cfg.LogPath) {
		absLogPath, err := filepath.Abs(cfg.LogPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve local Flask log path: %w", err)
		}
		cfg.LogPath = absLogPath
	}
	if cfg.BackendBaseURL == "" {
		cfg.BackendBaseURL = "http://127.0.0.1:8000"
	}

	if _, err := os.Stat(cfg.AppPath); err != nil {
		return nil, fmt.Errorf("local Flask app.py is not accessible: %w", err)
	}
	if err := ensurePortAvailable("0.0.0.0", cfg.Port); err != nil {
		return nil, err
	}

	pythonPath, err := findPython()
	if err != nil {
		return nil, err
	}

	depCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := checkLocalFlaskDependencies(depCtx, pythonPath); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open local Flask log file: %w", err)
	}
	defer logFile.Close()

	_, _ = fmt.Fprintf(logFile, "\n[%s] starting local Flask app on 0.0.0.0:%d\n", time.Now().Format(time.RFC3339), cfg.Port)

	cmd := exec.Command(pythonPath, cfg.AppPath)
	cmd.Dir = cfg.AppDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"APP_HOST=0.0.0.0",
		fmt.Sprintf("PORT=%d", cfg.Port),
		"PYTHONUNBUFFERED=1",
		"BACKEND_BASE_URL="+strings.TrimRight(cfg.BackendBaseURL, "/"),
		fmt.Sprintf("USE_HTTPS=%t", cfg.UseHTTPS),
		"CONTAINER_SSL_CERT_PATH="+cfg.SSLCertPath,
		"CONTAINER_SSL_KEY_PATH="+cfg.SSLKeyPath,
	)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start local Flask app: %w", err)
	}

	process := &LocalFlaskProcess{
		PID:        cmd.Process.Pid,
		PythonPath: pythonPath,
	}

	if err := cmd.Process.Release(); err != nil {
		return nil, fmt.Errorf("failed to release local Flask process: %w", err)
	}

	if err := waitForPortOpen("127.0.0.1", cfg.Port, 3*time.Second); err != nil {
		_ = StopLocalProcess(process.PID)
		return nil, err
	}

	return process, nil
}

func StopLocalProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid local process id")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find local process %d: %w", pid, err)
	}

	_ = process.Signal(os.Interrupt)
	time.Sleep(700 * time.Millisecond)
	_ = process.Kill()

	return nil
}

func findPython() (string, error) {
	for _, candidate := range []string{"python3", "python"} {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python3 or python was not found in PATH")
}

func checkLocalFlaskDependencies(ctx context.Context, pythonPath string) error {
	cmd := exec.CommandContext(ctx, pythonPath, "-c", "import flask, flask_cors, requests")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = err.Error()
	}

	return fmt.Errorf("local Python is missing Flask runtime dependencies; run %s -m pip install -r requirements.txt in the generated app directory: %s", filepath.Base(pythonPath), detail)
}

func ensurePortAvailable(host string, port uint) error {
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("local port %d is unavailable: %w", port, err)
	}
	return listener.Close()
}

func IsTCPPortOpen(host string, port uint, timeout time.Duration) bool {
	if port == 0 {
		return false
	}
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForPortOpen(host string, port uint, timeout time.Duration) error {
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	return fmt.Errorf("local Flask process started but port %d did not become ready", port)
}
