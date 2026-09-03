package utils

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const RequiredProjectBaseImage = "python:3.9-slim"

// DockerClient wraps of Docker client
type DockerClient struct {
	*client.Client
}

type containerHTTPSConfig struct {
	Enabled           bool
	HostCertPath      string
	HostKeyPath       string
	ContainerCertPath string
	ContainerKeyPath  string
}

// NewDockerClient creates a new Docker client
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	return &DockerClient{Client: cli}, nil
}

// ImageExists reports whether an image is already available in the local Docker daemon.
func (d *DockerClient) ImageExists(ctx context.Context, image string) (bool, error) {
	_, _, err := d.Client.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return true, nil
	}
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect Docker image %s: %w", image, err)
}

// PullImage downloads an image and streams Docker's progress to output.
func (d *DockerClient) PullImage(ctx context.Context, imageRef string, output io.Writer) error {
	response, err := d.Client.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull Docker image %s: %w", imageRef, err)
	}
	defer response.Close()
	if _, err := io.Copy(output, response); err != nil {
		return fmt.Errorf("failed to read Docker pull progress for %s: %w", imageRef, err)
	}
	return nil
}

// BuildImage builds a Docker image from a directory
func (d *DockerClient) BuildImage(ctx context.Context, buildContext string, imageName string) error {
	// Create tar archive from build context
	archive, err := createBuildContext(buildContext)
	if err != nil {
		return fmt.Errorf("failed to create build context archive: %w", err)
	}
	defer os.Remove(archive.Name())

	// Seek to beginning of archive
	archive.Seek(0, 0)

	// Build Docker image
	buildOptions := types.ImageBuildOptions{
		Dockerfile:  "Dockerfile",
		Tags:        []string{imageName},
		Remove:      true,
		ForceRemove: true,
		NoCache:     false,
	}

	buildResponse, err := d.Client.ImageBuild(ctx, archive, buildOptions)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer buildResponse.Body.Close()

	// Read and log build output
	_, err = io.Copy(os.Stdout, buildResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read build output: %w", err)
	}

	return nil
}

// createBuildContext creates a tar archive of build context
func createBuildContext(buildContext string) (*os.File, error) {
	archive, err := os.CreateTemp("", "docker-build-*.tar")
	if err != nil {
		return nil, err
	}

	tw := tar.NewWriter(archive)

	// Explicitly add each file/directory
	filesToAdd := []string{
		"Dockerfile",
		"app.py",
		"requirements.txt",
		"templates",
		"templates/index.html",
		"static",
	}

	for _, fileRelPath := range filesToAdd {
		fullPath := filepath.Join(buildContext, fileRelPath)
		fileInfo, err := os.Stat(fullPath)
		if err != nil {
			fmt.Printf("[DEBUG] Skipping %s: %v\n", fileRelPath, err)
			continue
		}

		// Create tar header
		header, err := tar.FileInfoHeader(fileInfo, "")
		if err != nil {
			fmt.Printf("[DEBUG] Error creating header for %s: %v\n", fileRelPath, err)
			continue
		}

		// Ensure forward slashes
		header.Name = filepath.ToSlash(fileRelPath)

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			fmt.Printf("[DEBUG] Error writing header for %s: %v\n", fileRelPath, err)
			continue
		}

		// Write file content for regular files
		if !fileInfo.IsDir() && fileInfo.Mode().IsRegular() {
			file, err := os.Open(fullPath)
			if err != nil {
				fmt.Printf("[DEBUG] Error opening %s: %v\n", fileRelPath, err)
				continue
			}

			_, err = io.Copy(tw, file)
			file.Close()

			if err != nil {
				fmt.Printf("[DEBUG] Error writing content for %s: %v\n", fileRelPath, err)
				continue
			}
			fmt.Printf("[DEBUG] Added %s to tar\n", fileRelPath)
		} else {
			fmt.Printf("[DEBUG] Added directory %s to tar\n", fileRelPath)
		}
	}

	// Close tar writer to flush all data
	if err := tw.Close(); err != nil {
		os.Remove(archive.Name())
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Sync to ensure data is written
	archive.Sync()

	return archive, nil
}

func loadContainerHTTPSConfig(useHTTPS bool, hostCertOverride string, hostKeyOverride string) (containerHTTPSConfig, error) {
	httpsCfg := containerHTTPSConfig{
		Enabled:           useHTTPS,
		ContainerCertPath: strings.TrimSpace(config.ContainerSSLCertPath()),
		ContainerKeyPath:  strings.TrimSpace(config.ContainerSSLKeyPath()),
	}

	if httpsCfg.ContainerCertPath == "" {
		httpsCfg.ContainerCertPath = "/app/certificates/cert.pem"
	}
	if httpsCfg.ContainerKeyPath == "" {
		httpsCfg.ContainerKeyPath = "/app/certificates/key.pem"
	}

	if !httpsCfg.Enabled {
		return httpsCfg, nil
	}

	httpsCfg.HostCertPath = strings.TrimSpace(hostCertOverride)
	httpsCfg.HostKeyPath = strings.TrimSpace(hostKeyOverride)
	if httpsCfg.HostCertPath == "" || httpsCfg.HostKeyPath == "" {
		return httpsCfg, fmt.Errorf("HTTPS is enabled for the project, but SSL certificate or key file is missing")
	}

	absCertPath, err := filepath.Abs(httpsCfg.HostCertPath)
	if err != nil {
		return httpsCfg, fmt.Errorf("failed to resolve project SSL certificate path: %w", err)
	}
	absKeyPath, err := filepath.Abs(httpsCfg.HostKeyPath)
	if err != nil {
		return httpsCfg, fmt.Errorf("failed to resolve project SSL key path: %w", err)
	}

	if _, err := os.Stat(absCertPath); err != nil {
		return httpsCfg, fmt.Errorf("project SSL certificate file is not accessible: %w", err)
	}
	if _, err := os.Stat(absKeyPath); err != nil {
		return httpsCfg, fmt.Errorf("project SSL key file is not accessible: %w", err)
	}

	httpsCfg.HostCertPath = absCertPath
	httpsCfg.HostKeyPath = absKeyPath
	return httpsCfg, nil
}

func (d *DockerClient) removeExistingProjectContainers(ctx context.Context, projectID uint, containerName string) error {
	expectedNames := map[string]struct{}{
		fmt.Sprintf("/%s", containerName):           {},
		fmt.Sprintf("/container-app-%d", projectID): {},
		fmt.Sprintf("/flask-app-%d", projectID):     {},
	}
	projectPrefix := fmt.Sprintf("/%d-", projectID)

	containers, err := d.Client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list existing containers: %w", err)
	}

	for _, existingContainer := range containers {
		shouldRemove := false
		for _, existingName := range existingContainer.Names {
			if _, ok := expectedNames[existingName]; ok || strings.HasPrefix(existingName, projectPrefix) {
				shouldRemove = true
				break
			}
		}

		if !shouldRemove {
			continue
		}

		if err := d.Client.ContainerRemove(ctx, existingContainer.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("failed to remove existing container %s: %w", existingContainer.ID, err)
		}
	}

	return nil
}

// CreateContainer creates a container from an image
func (d *DockerClient) CreateContainer(ctx context.Context, imageName string, port int, projectID uint, useHTTPS bool, hostCertOverride string, hostKeyOverride string) (string, error) {
	containerName := imageName
	httpsConfig, err := loadContainerHTTPSConfig(useHTTPS, hostCertOverride, hostKeyOverride)
	if err != nil {
		return "", err
	}

	containerPort := "5000"
	if httpsConfig.Enabled {
		containerPort = "443"
	}

	// Remove current and legacy project containers so the new naming scheme is applied consistently.
	if err := d.removeExistingProjectContainers(ctx, projectID, containerName); err != nil {
		return "", err
	}

	// Create container
	containerEnv := []string{
		"BACKEND_BASE_URL=http://host.docker.internal:8000",
		fmt.Sprintf("USE_HTTPS=%t", httpsConfig.Enabled),
		fmt.Sprintf("CONTAINER_SSL_CERT_PATH=%s", httpsConfig.ContainerCertPath),
		fmt.Sprintf("CONTAINER_SSL_KEY_PATH=%s", httpsConfig.ContainerKeyPath),
	}

	binds := []string{}
	if httpsConfig.Enabled {
		binds = append(binds,
			fmt.Sprintf("%s:%s:ro", httpsConfig.HostCertPath, httpsConfig.ContainerCertPath),
			fmt.Sprintf("%s:%s:ro", httpsConfig.HostKeyPath, httpsConfig.ContainerKeyPath),
		)
	}

	resp, err := d.Client.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		Env:   containerEnv,
		ExposedPorts: nat.PortSet{
			nat.Port(fmt.Sprintf("%s/tcp", containerPort)): struct{}{},
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(fmt.Sprintf("%s/tcp", containerPort)): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", port),
				},
			},
		},
		Binds: binds,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
	}, nil, nil, containerName)

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

// StartContainer starts a container
func (d *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	return d.Client.ContainerStart(ctx, containerID, container.StartOptions{})
}

// StopContainer stops a container
func (d *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10
	return d.Client.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &timeout,
	})
}

// RemoveContainer removes a container
func (d *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	return d.Client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force: true,
	})
}

// GetContainerStatus returns status of a container
func (d *DockerClient) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	containerJSON, err := d.Client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	if containerJSON.State.Running {
		return "running", nil
	}

	return containerJSON.State.Status, nil
}

// GetContainerLogs returns logs of a container
func (d *DockerClient) GetContainerLogs(ctx context.Context, containerID string) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       "100",
	}

	reader, err := d.Client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer reader.Close()

	payload, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, bytes.NewReader(payload)); err != nil {
		return string(payload), nil
	}

	if stderrBuf.Len() == 0 {
		return stdoutBuf.String(), nil
	}

	if stdoutBuf.Len() == 0 {
		return stderrBuf.String(), nil
	}

	return stdoutBuf.String() + "\n" + stderrBuf.String(), nil
}
