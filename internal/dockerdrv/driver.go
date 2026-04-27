package dockerdrv

import (
	"context"
	"errors"
	"io"
	"time"
)

//go:generate mockgen -source=driver.go -destination=mocks/mock_driver.go -package=mocks

var (
	// ErrContainerNotFound is returned by Stop, Remove, Start, Logs, ContainerIP
	// when the named container does not exist.
	ErrContainerNotFound = errors.New("dockerdrv: container not found")

	// ErrNoBridgeIP is returned by ContainerIP when docker inspect returns an
	// empty IP string. Typically transient; the readiness probe re-resolves
	// on the next tick.
	ErrNoBridgeIP = errors.New("dockerdrv: container has no bridge network IP")
)

type BuildRequest struct {
	ImageRef   string
	SourceDir  string
	Dockerfile string
	Stdout     io.Writer
	Stderr     io.Writer
}

type RunRequest struct {
	Name    string
	Image   string
	Network string
	Env     map[string]string
	Restart string
	Port    int
}

type InspectResult struct {
	ContainerID string
	State       string // "running" | "exited" | "absent"
}

type LogsOptions struct {
	Follow bool
	Tail   int
	Stdout io.Writer
	Stderr io.Writer
}

// PortMap describes a single host:container port publish, including the host
// bind interface. Dual-stack publishing is expressed by emitting one PortMap
// for "0.0.0.0" and another for "[::]" for the same container port. The
// driver's RunWithOptions emits one `-p` flag per PortMap, in declared order.
type PortMap struct {
	HostBind      string // "0.0.0.0", "[::]", or "" (Docker default)
	HostPort      int
	ContainerPort int
	Proto         string // "tcp" or "udp"
}

// VolumeMount describes a single -v flag. Bind mounts use Source as a host
// directory path; named volumes use Source as the volume name and IsNamed=true.
type VolumeMount struct {
	Source   string
	Target   string
	ReadOnly bool
	IsNamed  bool
}

// RunOptions is the full-control variant of RunRequest. Service deploys keep
// using Run(RunRequest); RunWithOptions is for callers that need port
// publishing, volume mounts, or labels (currently: caddy.Manager).
type RunOptions struct {
	Name    string
	Image   string
	Network string
	Restart string
	Ports   []PortMap         // emitted in declared order, one -p per entry
	Volumes []VolumeMount     // emitted in declared order, one -v per entry
	Labels  map[string]string // emitted with sorted keys
	Env     map[string]string // emitted with sorted keys
}

// ExecOptions describes a `docker exec` invocation. Stdin is never wired;
// -i/-t are never passed.
type ExecOptions struct {
	Container string
	Cmd       []string
	Stdout    io.Writer
	Stderr    io.Writer
}

type Driver interface {
	Build(ctx context.Context, req BuildRequest) (imageID string, err error)
	Run(ctx context.Context, req RunRequest) (containerID string, err error)
	Stop(ctx context.Context, containerName string, gracePeriod time.Duration) error
	Start(ctx context.Context, containerName string) error
	Remove(ctx context.Context, containerName string) error
	Inspect(ctx context.Context, containerName string) (InspectResult, error)
	Logs(ctx context.Context, containerName string, opts LogsOptions) error
	NetworkEnsure(ctx context.Context, networkName string) error
	ContainerIP(ctx context.Context, containerName string) (string, error)

	// ImagePull invokes `docker pull <ref>`.
	ImagePull(ctx context.Context, ref string) error

	// RunWithOptions invokes `docker run -d` with the full set of flags
	// described by opts. Returns the trimmed stdout (the container ID).
	RunWithOptions(ctx context.Context, opts RunOptions) (containerID string, err error)

	// Exec invokes `docker exec <container> <cmd...>`. Returns
	// ErrContainerNotFound when the container does not exist.
	Exec(ctx context.Context, opts ExecOptions) error
}
