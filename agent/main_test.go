package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeployServesRouteAndProxiesSubmission(t *testing.T) {
	var submissions atomic.Int64
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifact/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<html><body>distributed page</body></html>`)
		case strings.HasSuffix(r.URL.Path, "/submit/"):
			submissions.Add(1)
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "username=tester") {
				t.Errorf("unexpected submission: %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"message":"ok"}`)
		case strings.HasSuffix(r.URL.Path, "/logs/"):
			_, _ = io.WriteString(w, `{"accepted":1,"acked_sequence":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer controller.Close()

	a := &Agent{
		config: Config{ServerURL: controller.URL, WorkDir: t.TempDir(), BindHost: "127.0.0.1", AdvertiseHost: "127.0.0.1"},
		state:  State{AgentID: "agent-test", Token: "token"}, client: controller.Client(),
		runs: make(map[uint]*deploymentRuntime), seq: make(map[uint]int64),
	}
	task := Task{TaskID: "task-1", Type: "project.deploy", ProjectID: 7, DeploymentID: 11}
	task.Payload.Revision = 2
	task.Payload.FrontendRoute = "/training"
	task.Payload.ContainerRoute = "/api/submit"
	task.Payload.ArtifactEndpoint = "/api/agent/deployments/11/artifact/"
	portProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	task.Payload.RuntimePort = uint(portProbe.Addr().(*net.TCPAddr).Port)
	_ = portProbe.Close()
	_, runtimeURL, err := a.deploy(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	defer a.stopAll()

	page, err := http.Get(runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK || !strings.Contains(string(pageBody), "distributed page") {
		t.Fatalf("page response: %d %s", page.StatusCode, pageBody)
	}
	response, err := http.Post(strings.TrimSuffix(runtimeURL, "/training")+"/api/submit", "application/x-www-form-urlencoded", strings.NewReader("username=tester&password=value"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("submit status = %d", response.StatusCode)
	}
	if submissions.Load() != 1 {
		t.Fatalf("submissions = %d, want 1", submissions.Load())
	}
}
