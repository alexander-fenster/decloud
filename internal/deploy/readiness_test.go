package deploy_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/dockerdrv"
	dockermocks "github.com/alexander-fenster/decloud/internal/dockerdrv/mocks"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const probeContainerName = "decloud-foo"

func startTestServer(t *testing.T, h http.HandlerFunc) (host string, port int) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host = u.Hostname()
	p, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return host, p
}

func newReadinessSpec() registry.ReadinessSpec {
	return registry.ReadinessSpec{
		Kind:         "http",
		HTTPPath:     "/healthz",
		TimeoutSecs:  2,
		IntervalSecs: 1,
	}
}

func newProbeDriver(t *testing.T) *dockermocks.MockDriver {
	t.Helper()
	return dockermocks.NewMockDriver(gomock.NewController(t))
}

func TestReadiness_HTTPSuccessReturnsNil(t *testing.T) {
	host, port := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return(host, nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	require.NoError(t, probe.Wait(context.Background(), probeContainerName, newReadinessSpec(), port))
}

func TestReadiness_HTTPSuccessWithIPv6HostReturnsNil(t *testing.T) {
	listener, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)

	_, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return("::1", nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	require.NoError(t, probe.Wait(context.Background(), probeContainerName, newReadinessSpec(), port))
}

func TestReadiness_RedirectWithMalformedLocationCountsAsReady(t *testing.T) {
	host, port := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://invalid IP:8080/healthz")
		w.WriteHeader(http.StatusFound)
	})
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return(host, nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	require.NoError(t, probe.Wait(context.Background(), probeContainerName, newReadinessSpec(), port))
}

func TestReadiness_ContainerIPLookupFailureReturnsErrReadiness(t *testing.T) {
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).
		Return("", errors.New("inspect failed")).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.TimeoutSecs = 1
	err := probe.Wait(context.Background(), probeContainerName, spec, 9999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrReadiness))
}

func TestReadiness_ContainerIPInitiallyEmptyThenReady(t *testing.T) {
	host, port := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	driver := newProbeDriver(t)
	var calls atomic.Int32
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).
		DoAndReturn(func(_ context.Context, _ string) (string, error) {
			if calls.Add(1) == 1 {
				return "", dockerdrv.ErrNoBridgeIP
			}
			return host, nil
		}).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.IntervalSecs = 1
	spec.TimeoutSecs = 5
	require.NoError(t, probe.Wait(context.Background(), probeContainerName, spec, port))
}

func TestReadiness_HTTPTimeoutReturnsErrReadiness(t *testing.T) {
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return("127.0.0.1", nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.TimeoutSecs = 1
	err := probe.Wait(context.Background(), probeContainerName, spec, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrReadiness))
}

func TestReadiness_HTTPRetriesOnTransientFailure(t *testing.T) {
	var hits atomic.Int32
	host, port := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return(host, nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.IntervalSecs = 1
	spec.TimeoutSecs = 5
	require.NoError(t, probe.Wait(context.Background(), probeContainerName, spec, port))
	assert.GreaterOrEqual(t, hits.Load(), int32(2), "must retry after transient 5xx")
}

func TestReadiness_EmptyIPNoErrorReportsErrNoBridgeIP(t *testing.T) {
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).
		Return("", nil).AnyTimes()

	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.TimeoutSecs = 1
	err := probe.Wait(context.Background(), probeContainerName, spec, 1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, deploy.ErrReadiness))
	assert.True(t, errors.Is(err, dockerdrv.ErrNoBridgeIP),
		"empty IP without driver error must surface as ErrNoBridgeIP through the readiness wrap")
}

func TestReadiness_ContextCancellationStopsProbe(t *testing.T) {
	host, port := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	driver := newProbeDriver(t)
	driver.EXPECT().ContainerIP(gomock.Any(), probeContainerName).Return(host, nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	probe := deploy.NewHTTPProbe(driver)
	spec := newReadinessSpec()
	spec.TimeoutSecs = 30
	err := probe.Wait(ctx, probeContainerName, spec, port)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled),
		"context cancellation must surface as context.Canceled in the error chain; got %v", err)
	assert.False(t, errors.Is(err, deploy.ErrReadiness),
		"context cancellation must NOT be wrapped as ErrReadiness; got %v", err)
}
