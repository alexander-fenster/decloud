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
}
