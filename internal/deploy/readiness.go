package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	"github.com/alexander-fenster/decloud/internal/registry"
)

// ReadinessProbe is the package's exported probe contract.
type ReadinessProbe interface {
	Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error
}

// NewHTTPProbe constructs the production HTTP readiness probe.
func NewHTTPProbe(driver dockerdrv.Driver) ReadinessProbe {
	return newHTTPProbe(driver)
}

type httpProbe struct {
	client *http.Client
	driver dockerdrv.Driver
}

func newHTTPProbe(driver dockerdrv.Driver) *httpProbe {
	return &httpProbe{
		client: &http.Client{Timeout: 2 * time.Second},
		driver: driver,
	}
}

func (p *httpProbe) Wait(ctx context.Context, containerName string, spec registry.ReadinessSpec, port int) error {
	interval := time.Duration(spec.IntervalSecs) * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	timeout := time.Duration(spec.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		ip, ipErr := p.driver.ContainerIP(ctx, containerName)
		switch {
		case ipErr != nil:
			lastErr = ipErr
		case ip == "":
			lastErr = dockerdrv.ErrNoBridgeIP
		default:
			url := fmt.Sprintf("http://%s:%d%s", ip, port, spec.HTTPPath)
			if err := p.probeOnce(ctx, url); err != nil {
				lastErr = err
			} else {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("%w: %w", ErrReadiness, lastErr)
			}
			return fmt.Errorf("%w: timed out after %s", ErrReadiness, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (p *httpProbe) probeOnce(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return errors.New("non-2xx/3xx status")
}
