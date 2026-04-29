package dockerdrv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cmdFactory is the test seam for shelling out. Production uses
// exec.CommandContext; tests inject a recording factory that captures args
// without executing the real docker binary.
type cmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

type cliDriver struct {
	cmd cmdFactory
}

// NewCLIDriver returns the production driver that shells out to `docker`.
func NewCLIDriver() Driver {
	return &cliDriver{cmd: exec.CommandContext}
}

// newCLIDriverWithFactory is the test constructor.
func newCLIDriverWithFactory(f cmdFactory) Driver {
	return &cliDriver{cmd: f}
}

func (d *cliDriver) Build(ctx context.Context, req BuildRequest) (string, error) {
	args := []string{"build", "-t", req.ImageRef, "-f", req.Dockerfile, req.SourceDir}
	cmd := d.cmd(ctx, "docker", args...)
	cmd.Stdout = req.Stdout
	cmd.Stderr = req.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build: %w", err)
	}
	return req.ImageRef, nil
}

func (d *cliDriver) Run(ctx context.Context, req RunRequest) (string, error) {
	args := []string{
		"run", "-d",
		"--name", req.Name,
		"--network", req.Network,
		"--restart", req.Restart,
	}
	keys := make([]string, 0, len(req.Env))
	for k := range req.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--env", k+"="+req.Env[k])
	}
	for _, v := range req.Volumes {
		args = append(args, "-v", formatVolume(v))
	}
	args = append(args, "--label", "decloud.service="+strings.TrimPrefix(req.Name, "decloud-"))
	args = append(args, req.Image)

	var stdout, stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (d *cliDriver) Stop(ctx context.Context, name string, grace time.Duration) error {
	args := []string{"stop", "-t", strconv.Itoa(int(grace.Seconds())), name}
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return ErrContainerNotFound
		}
		return fmt.Errorf("docker stop: %w; stderr=%q", err, stderr.String())
	}
	return nil
}

func (d *cliDriver) Start(ctx context.Context, name string) error {
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "start", name)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return ErrContainerNotFound
		}
		return fmt.Errorf("docker start: %w; stderr=%q", err, stderr.String())
	}
	return nil
}

func (d *cliDriver) Remove(ctx context.Context, name string) error {
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "rm", name)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return ErrContainerNotFound
		}
		return fmt.Errorf("docker rm: %w; stderr=%q", err, stderr.String())
	}
	return nil
}

func (d *cliDriver) Inspect(ctx context.Context, name string) (InspectResult, error) {
	const formatArg = `{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}`
	var stdout, stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "inspect", name, "--format", formatArg)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return InspectResult{State: "absent"}, nil
		}
		return InspectResult{}, fmt.Errorf("docker inspect: %w; stderr=%q", err, stderr.String())
	}
	var parsed struct {
		ID     string            `json:"id"`
		State  string            `json:"state"`
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &parsed); err != nil {
		return InspectResult{}, fmt.Errorf("docker inspect: parse output %q: %w", stdout.String(), err)
	}
	return InspectResult{ContainerID: parsed.ID, State: parsed.State, Labels: parsed.Labels}, nil
}

func (d *cliDriver) Logs(ctx context.Context, name string, opts LogsOptions) error {
	args := []string{"logs"}
	if opts.Follow {
		args = append(args, "-f")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(opts.Tail))
	}
	args = append(args, name)
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", args...)
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = io.MultiWriter(opts.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return ErrContainerNotFound
		}
		return fmt.Errorf("docker logs: %w; stderr=%q", err, stderr.String())
	}
	return nil
}

func (d *cliDriver) NetworkEnsure(ctx context.Context, name string) error {
	if err := d.cmd(ctx, "docker", "network", "inspect", name).Run(); err == nil {
		return nil
	}
	if err := d.cmd(ctx, "docker", "network", "create", name).Run(); err != nil {
		return fmt.Errorf("docker network create: %w", err)
	}
	return nil
}

func (d *cliDriver) ContainerIP(ctx context.Context, name string) (string, error) {
	const formatArg = "{{ .NetworkSettings.Networks.decloud.IPAddress }}"
	var stdout, stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "inspect", name, "--format", formatArg)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return "", ErrContainerNotFound
		}
		return "", fmt.Errorf("docker inspect ip: %w; stderr=%q", err, stderr.String())
	}
	ip := strings.TrimSpace(stdout.String())
	if ip == "" {
		return "", ErrNoBridgeIP
	}
	return ip, nil
}

func isNotFound(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "no such container") || strings.Contains(s, "no such object")
}

func (d *cliDriver) ImagePull(ctx context.Context, ref string) error {
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "pull", ref)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull %s: %w; stderr=%q", ref, err, stderr.String())
	}
	return nil
}

func (d *cliDriver) RunWithOptions(ctx context.Context, opts RunOptions) (string, error) {
	args := []string{
		"run", "-d",
		"--name", opts.Name,
		"--network", opts.Network,
		"--restart", opts.Restart,
	}
	envKeys := make([]string, 0, len(opts.Env))
	for k := range opts.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		args = append(args, "--env", k+"="+opts.Env[k])
	}
	labelKeys := make([]string, 0, len(opts.Labels))
	for k := range opts.Labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	for _, k := range labelKeys {
		args = append(args, "--label", k+"="+opts.Labels[k])
	}
	for _, p := range opts.Ports {
		args = append(args, "-p", formatPortMap(p))
	}
	for _, v := range opts.Volumes {
		args = append(args, "-v", formatVolume(v))
	}
	args = append(args, opts.Image)

	var stdout, stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker run: %w; stderr=%q", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (d *cliDriver) Exec(ctx context.Context, opts ExecOptions) error {
	args := append([]string{"exec", opts.Container}, opts.Cmd...)
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", args...)
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = io.MultiWriter(opts.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		if isNotFound(stderr.String()) {
			return ErrContainerNotFound
		}
		return fmt.Errorf("docker exec: %w; stderr=%q", err, stderr.String())
	}
	return nil
}

// formatPortMap renders <HostBind>:<HostPort>:<ContainerPort>/<Proto>. HostBind
// is spliced literally — IPv6 callers MUST pass "[::]" already-bracketed.
// Auto-bracketing here would double-bracket IPv6 and break IPv4.
func formatPortMap(p PortMap) string {
	proto := p.Proto
	if proto == "" {
		proto = "tcp"
	}
	if p.HostBind == "" {
		return fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, proto)
	}
	return fmt.Sprintf("%s:%d:%d/%s", p.HostBind, p.HostPort, p.ContainerPort, proto)
}

func formatVolume(v VolumeMount) string {
	s := v.Source + ":" + v.Target
	if v.ReadOnly {
		s += ":ro"
	}
	return s
}
