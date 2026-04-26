package cli

import (
	"context"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/cli/mocks"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func installMockLifecycle(t *testing.T) *mocks.MockLifecycle {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := mocks.NewMockLifecycle(ctrl)
	prev := lifecycleFactory
	lifecycleFactory = func(_ config.Paths) (deploy.Lifecycle, error) { return mock, nil }
	t.Cleanup(func() { lifecycleFactory = prev })
	return mock
}

func TestUnregister_DelegatesToLifecycle(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().Unregister(gomock.Any(), "foo").Return(nil)

	_, _, err := runRoot(t, "unregister", "foo")
	require.NoError(t, err)
}

func TestStart_DelegatesToLifecycle(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().Start(gomock.Any(), "foo").Return(nil)

	_, _, err := runRoot(t, "start", "foo")
	require.NoError(t, err)
}

func TestStop_DelegatesToLifecycle(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().Stop(gomock.Any(), "foo").Return(nil)

	_, _, err := runRoot(t, "stop", "foo")
	require.NoError(t, err)
}

func TestRestart_DelegatesToLifecycle(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().Restart(gomock.Any(), "foo").Return(nil)

	_, _, err := runRoot(t, "restart", "foo")
	require.NoError(t, err)
}

func TestStatus_DelegatesToLifecycleAndPrintsResult(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().Status(gomock.Any(), "foo").Return(deploy.Status{
		Name:           "foo",
		ContainerID:    "cid",
		ContainerName:  "decloud-foo",
		State:          "running",
		LastDeployID:   "20260426-120000-abc123",
		LastDeployedAt: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC),
	}, nil)

	stdout, _, err := runRoot(t, "status", "foo")
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "foo")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, "decloud-foo")
}

func TestLogs_DelegatesToLifecycleWithFlags(t *testing.T) {
	mock := installMockLifecycle(t)
	var seen deploy.LogOptions
	mock.EXPECT().Logs(gomock.Any(), "foo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, opts deploy.LogOptions) error {
			seen = opts
			return nil
		})

	_, _, err := runRoot(t, "logs", "--follow", "--tail", "100", "foo")
	require.NoError(t, err)
	assert.True(t, seen.Follow)
	assert.Equal(t, 100, seen.Tail)
}

func TestCaddyReload_DelegatesToLifecycle(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().CaddyReload(gomock.Any()).Return(nil)

	_, _, err := runRoot(t, "caddy", "reload")
	require.NoError(t, err)
}
