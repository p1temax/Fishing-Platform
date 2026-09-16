package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const version = "1.0.0"

type Config struct {
	ServerURL           string   `yaml:"server_url"`
	RegistrationToken   string   `yaml:"registration_token"`
	AgentID             string   `yaml:"agent_id"`
	Name                string   `yaml:"name"`
	WorkDir             string   `yaml:"work_dir"`
	BindHost            string   `yaml:"bind_host"`
	AdvertiseHost       string   `yaml:"advertise_host"`
	HeartbeatInterval   string   `yaml:"heartbeat_interval"`
	PollInterval        string   `yaml:"poll_interval"`
	LogBufferMaxEntries int      `yaml:"log_buffer_max_entries"`
	Tags                []string `yaml:"tags"`
}

type State struct {
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}

type Task struct {
	TaskID       string `json:"task_id"`
	Type         string `json:"type"`
	ProjectID    uint   `json:"project_id"`
	DeploymentID uint   `json:"deployment_id"`
	Payload      struct {
		Revision         int64  `json:"revision"`
		ArtifactEndpoint string `json:"artifact_endpoint"`
		FrontendRoute    string `json:"frontend_route"`
		ContainerRoute   string `json:"container_route"`
		RuntimePort      uint   `json:"runtime_port"`
	} `json:"payload"`
}

type taskList struct {
	Tasks []Task `json:"tasks"`
}

type deploymentRuntime struct {
	server   *http.Server
	listener net.Listener
	port     uint
	dir      string
}

type bufferedLog struct {
	DeploymentID uint      `json:"deployment_id"`
	ProjectID    uint      `json:"project_id"`
	Sequence     int64     `json:"sequence"`
	TaskID       string    `json:"task_id"`
	Stream       string    `json:"stream"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	LoggedAt     time.Time `json:"logged_at"`
}

type Agent struct {
	config      Config
	state       State
	client      *http.Client
	mu          sync.Mutex
	runs        map[uint]*deploymentRuntime
	seq         map[uint]int64
	logMu       sync.Mutex
	flushMu     sync.Mutex
	pendingLogs []bufferedLog
}

func main() {
	configPath := flag.String("config", "agent.yaml", "agent configuration path")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o750); err != nil {
		log.Fatalf("create work directory: %v", err)
	}
	a := &Agent{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		runs:   make(map[uint]*deploymentRuntime),
		seq:    make(map[uint]int64),
	}
	if err := a.loadOrRegister(); err != nil {
		log.Fatal(err)
	}
	a.loadLogBuffer()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("agent %s (%s) connected to %s", a.config.Name, a.state.AgentID, a.config.ServerURL)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); a.heartbeatLoop(ctx) }()
	go func() { defer wg.Done(); a.taskLoop(ctx) }()
	go func() { defer wg.Done(); a.logLoop(ctx) }()
	<-ctx.Done()
	a.stopAll()
	a.flushLogs()
	wg.Wait()
}

func (a *Agent) logLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.flushLogs()
		}
	}
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.ServerURL = strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/")
	if cfg.ServerURL == "" {
		return Config{}, errors.New("server_url is required")
	}
	if cfg.Name == "" {
		hostname, _ := os.Hostname()
		cfg.Name = hostname
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = "./data/agent"
	}
	if cfg.BindHost == "" {
		cfg.BindHost = "0.0.0.0"
	}
	if cfg.AdvertiseHost == "" {
		cfg.AdvertiseHost, _ = os.Hostname()
	}
	if cfg.HeartbeatInterval == "" {
		cfg.HeartbeatInterval = "30s"
	}
	if cfg.PollInterval == "" {
		cfg.PollInterval = "3s"
	}
	if cfg.LogBufferMaxEntries <= 0 {
		cfg.LogBufferMaxEntries = 10000
	}
	return cfg, nil
}

func (a *Agent) statePath() string { return filepath.Join(a.config.WorkDir, "state.json") }

func (a *Agent) loadOrRegister() error {
	data, err := os.ReadFile(a.statePath())
	if err == nil && json.Unmarshal(data, &a.state) == nil && a.state.Token != "" {
		return nil
	}
	if a.config.RegistrationToken == "" {
		return errors.New("registration_token is required for first registration")
	}
	hostname, _ := os.Hostname()
	payload := map[string]any{
		"registration_token": a.config.RegistrationToken,
		"agent_id":           a.config.AgentID, "name": a.config.Name, "hostname": hostname,
		"os": runtime.GOOS, "arch": runtime.GOARCH, "version": version,
		"capabilities": []string{"static-page", "central-logs"}, "tags": a.config.Tags,
	}
	var response struct {
		Token string `json:"token"`
		Agent struct {
			AgentID string `json:"agent_id"`
		} `json:"agent"`
	}
	if err := a.requestJSON(http.MethodPost, "/api/agents/register/", "", payload, &response); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	a.state = State{AgentID: response.Agent.AgentID, Token: response.Token}
	data, _ = json.MarshalIndent(a.state, "", "  ")
	if err := os.WriteFile(a.statePath(), data, 0o600); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	interval, _ := time.ParseDuration(a.config.HeartbeatInterval)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		payload := map[string]any{"name": a.config.Name, "version": version, "os": runtime.GOOS, "arch": runtime.GOARCH,
			"capabilities": []string{"static-page", "central-logs"}, "tags": a.config.Tags,
			"deployment_ids": a.runningDeploymentIDs()}
		if err := a.requestJSON(http.MethodPost, "/api/agents/heartbeat/", a.state.Token, payload, nil); err != nil {
			log.Printf("heartbeat failed: %v", err)
		}
		a.flushLogs()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *Agent) taskLoop(ctx context.Context) {
	interval, _ := time.ParseDuration(a.config.PollInterval)
	if interval <= 0 {
		interval = 3 * time.Second
	}
	for {
		var response taskList
		if err := a.requestJSON(http.MethodGet, "/api/agent/tasks/lease/?limit=5", a.state.Token, nil, &response); err != nil {
			log.Printf("lease tasks failed: %v", err)
		} else {
			for _, task := range response.Tasks {
				if err := a.executeTask(ctx, task); err != nil {
					log.Printf("task %s failed: %v", task.TaskID, err)
				}
			}
			a.flushLogs()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (a *Agent) executeTask(ctx context.Context, task Task) error {
	if err := a.requestJSON(http.MethodPost, "/api/agent/tasks/"+task.TaskID+"/start/", a.state.Token, map[string]any{}, nil); err != nil {
		return err
	}
	renewCtx, cancelRenew := context.WithCancel(ctx)
	var renewWG sync.WaitGroup
	renewWG.Add(1)
	go func() {
		defer renewWG.Done()
		a.renewTaskLease(renewCtx, task.TaskID)
	}()
	defer func() { cancelRenew(); renewWG.Wait() }()
	a.sendLog(task, "system", "info", "task started: "+task.Type)
	result := map[string]any{"success": true, "revision": task.Payload.Revision}
	var err error
	switch task.Type {
	case "project.deploy":
		var port uint
		var runtimeURL string
		port, runtimeURL, err = a.deploy(ctx, task)
		result["runtime_port"] = port
		result["runtime_url"] = runtimeURL
		result["container_name"] = fmt.Sprintf("fishing-d%d-p%d", task.DeploymentID, task.ProjectID)
	case "project.stop":
		err = a.stopDeployment(task.DeploymentID)
	case "project.delete":
		err = a.remove(task.DeploymentID)
	default:
		err = fmt.Errorf("unknown task type %q", task.Type)
	}
	if err != nil {
		result["success"] = false
		result["error"] = err.Error()
		a.sendLog(task, "system", "error", err.Error())
	} else {
		a.sendLog(task, "system", "info", "task completed: "+task.Type)
	}
	completeErr := a.requestJSON(http.MethodPost, "/api/agent/tasks/"+task.TaskID+"/complete/", a.state.Token, result, nil)
	if err != nil {
		return err
	}
	return completeErr
}

func (a *Agent) renewTaskLease(ctx context.Context, taskID string) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.requestJSON(http.MethodPost, "/api/agent/tasks/"+taskID+"/renew/", a.state.Token, map[string]any{}, nil); err != nil {
				log.Printf("renew task %s lease failed: %v", taskID, err)
			}
		}
	}
}

func (a *Agent) deploy(ctx context.Context, task Task) (uint, string, error) {
	dir := filepath.Join(a.config.WorkDir, "projects", fmt.Sprintf("deployment-%d", task.DeploymentID),
		"revisions", strconv.FormatInt(task.Payload.Revision, 10))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return 0, "", err
	}
	artifactURL := task.Payload.ArtifactEndpoint
	if artifactURL == "" {
		artifactURL = fmt.Sprintf("/api/agent/deployments/%d/artifact/", task.DeploymentID)
	}
	data, err := a.download(artifactURL)
	if err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), data, 0o640); err != nil {
		return 0, "", err
	}

	a.mu.Lock()
	if old := a.runs[task.DeploymentID]; old != nil {
		_ = old.server.Close()
		_ = old.listener.Close()
	}
	listener, err := a.listenProjectPort(task.Payload.RuntimePort)
	if err != nil {
		a.mu.Unlock()
		return 0, "", err
	}
	port := uint(listener.Addr().(*net.TCPAddr).Port)
	fileHandler := http.FileServer(http.Dir(dir))
	frontendRoute := "/" + strings.Trim(strings.TrimSpace(task.Payload.FrontendRoute), "/")
	if frontendRoute == "//" {
		frontendRoute = "/"
	}
	containerRoute := "/" + strings.Trim(strings.TrimSpace(task.Payload.ContainerRoute), "/")
	if containerRoute == "//" {
		containerRoute = "/api/submit"
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		if r.URL.Path == containerRoute && r.Method == http.MethodPost {
			a.proxySubmission(w, r, task)
		} else if r.URL.Path == frontendRoute || r.URL.Path == strings.TrimRight(frontendRoute, "/")+"/" {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
		} else {
			fileHandler.ServeHTTP(w, r)
		}
		a.sendLog(task, "access", "info", fmt.Sprintf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond)))
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	a.runs[task.DeploymentID] = &deploymentRuntime{server: server, listener: listener, port: port, dir: dir}
	a.mu.Unlock()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("deployment %d server: %v", task.DeploymentID, serveErr)
		}
	}()
	return port, fmt.Sprintf("http://%s:%d%s", a.config.AdvertiseHost, port, frontendRoute), nil
}

func (a *Agent) proxySubmission(w http.ResponseWriter, source *http.Request, task Task) {
	body, err := io.ReadAll(io.LimitReader(source.Body, 2<<20))
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	endpoint := fmt.Sprintf("%s/api/agent/deployments/%d/submit/", a.config.ServerURL, task.DeploymentID)
	req, err := http.NewRequestWithContext(source.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "proxy error", http.StatusBadGateway)
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.state.Token)
	req.Header.Set("Content-Type", source.Header.Get("Content-Type"))
	ip, _, splitErr := net.SplitHostPort(source.RemoteAddr)
	if splitErr != nil {
		ip = source.RemoteAddr
	}
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := a.client.Do(req)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}

func (a *Agent) remove(deploymentID uint) error {
	if err := a.stopDeployment(deploymentID); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(a.config.WorkDir, "projects", fmt.Sprintf("deployment-%d", deploymentID)))
}

func (a *Agent) listenProjectPort(port uint) (net.Listener, error) {
	if port == 0 {
		return nil, errors.New("runtime_port is required from controller")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(a.config.BindHost, strconv.Itoa(int(port))))
	if err != nil {
		return nil, fmt.Errorf("listen on controller-assigned port %d: %w", port, err)
	}
	return listener, nil
}

func (a *Agent) stopDeployment(deploymentID uint) error {
	a.mu.Lock()
	if current := a.runs[deploymentID]; current != nil {
		_ = current.server.Close()
		_ = current.listener.Close()
		delete(a.runs, deploymentID)
	}
	a.mu.Unlock()
	return nil
}

func (a *Agent) stopAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, current := range a.runs {
		_ = current.server.Close()
		_ = current.listener.Close()
	}
}

func (a *Agent) runningDeploymentIDs() []uint {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := make([]uint, 0, len(a.runs))
	for id := range a.runs {
		ids = append(ids, id)
	}
	return ids
}

func (a *Agent) sendLog(task Task, stream, level, message string) {
	a.mu.Lock()
	if a.seq[task.DeploymentID] == 0 {
		a.seq[task.DeploymentID] = time.Now().UnixNano()
	}
	a.seq[task.DeploymentID]++
	sequence := a.seq[task.DeploymentID]
	a.mu.Unlock()
	a.logMu.Lock()
	a.pendingLogs = append(a.pendingLogs, bufferedLog{
		DeploymentID: task.DeploymentID, ProjectID: task.ProjectID, Sequence: sequence,
		TaskID: task.TaskID, Stream: stream, Level: level, Message: message, LoggedAt: time.Now(),
	})
	maxEntries := a.config.LogBufferMaxEntries
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	if len(a.pendingLogs) > maxEntries {
		a.pendingLogs = a.pendingLogs[len(a.pendingLogs)-maxEntries:]
	}
	a.saveLogBufferLocked()
	a.logMu.Unlock()
}

func (a *Agent) logBufferPath() string { return filepath.Join(a.config.WorkDir, "pending-logs.json") }

func (a *Agent) loadLogBuffer() {
	data, err := os.ReadFile(a.logBufferPath())
	if err != nil {
		return
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if err := json.Unmarshal(data, &a.pendingLogs); err != nil {
		log.Printf("load pending logs: %v", err)
		a.pendingLogs = nil
	}
	for _, entry := range a.pendingLogs {
		if entry.Sequence > a.seq[entry.DeploymentID] {
			a.seq[entry.DeploymentID] = entry.Sequence
		}
	}
}

func (a *Agent) saveLogBufferLocked() {
	data, err := json.Marshal(a.pendingLogs)
	if err != nil {
		return
	}
	tmp := a.logBufferPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, a.logBufferPath())
	}
}

func (a *Agent) flushLogs() {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()
	for {
		a.logMu.Lock()
		if len(a.pendingLogs) == 0 {
			a.logMu.Unlock()
			return
		}
		deploymentID := a.pendingLogs[0].DeploymentID
		count := 0
		entries := make([]bufferedLog, 0, 100)
		for count < len(a.pendingLogs) && count < 100 && a.pendingLogs[count].DeploymentID == deploymentID {
			entries = append(entries, a.pendingLogs[count])
			count++
		}
		a.logMu.Unlock()
		payload := map[string]any{"entries": entries}
		if err := a.requestJSON(http.MethodPost, fmt.Sprintf("/api/agent/deployments/%d/logs/", deploymentID), a.state.Token, payload, nil); err != nil {
			log.Printf("upload buffered logs failed: %v", err)
			return
		}
		a.logMu.Lock()
		if len(a.pendingLogs) >= count && a.pendingLogs[0].Sequence == entries[0].Sequence {
			a.pendingLogs = a.pendingLogs[count:]
			a.saveLogBufferLocked()
		}
		a.logMu.Unlock()
	}
}

func (a *Agent) download(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, a.config.ServerURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.state.Token)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if expected := strings.TrimSpace(resp.Header.Get("X-Artifact-SHA256")); expected != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(expected, hex.EncodeToString(sum[:])) {
			return nil, errors.New("artifact checksum mismatch")
		}
	}
	return data, nil
}

func (a *Agent) requestJSON(method, path, token string, payload, target any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.config.ServerURL+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s: %s", path, resp.Status, strings.TrimSpace(string(data)))
	}
	if target != nil && len(data) > 0 {
		return json.Unmarshal(data, target)
	}
	return nil
}
