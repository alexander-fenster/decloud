package cli

import (
	"context"
	"errors"
	"strings"
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

	stdout, stderr, err := runRoot(t, "status", "foo")
	require.NoError(t, err)
	assert.Equal(t,
		"foo state=running container=decloud-foo deploy=20260426-120000-abc123 deployed_at=2026-04-26T12:00:00Z\n",
		stdout.String(),
		"single-service status output is a documented byte-for-byte contract; do not let the runStatusOne extraction drift it")
	assert.Empty(t, stderr.String(),
		"single-service path must not emit anything on stderr on success")
}

func runStatusNoArgs(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	stdoutBuf, stderrBuf, runErr := runRoot(t, "status")
	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func headerFields() []string {
	return []string{"NAME", "STATE", "CONTAINER", "DEPLOY", "DEPLOYED_AT"}
}

func assertHeaderPresent(t *testing.T, stdout string) {
	t.Helper()
	for _, field := range headerFields() {
		assert.Contains(t, stdout, field,
			"table header must include %q so columns are self-documenting", field)
	}
}

func assertRowPresent(t *testing.T, stdout, name, state string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name && fields[1] == state {
			return
		}
	}
	t.Fatalf("expected row for service %q with state %q in stdout:\n%s", name, state, stdout)
}

func assertBodyRowOrder(t *testing.T, stdout string, expectedNames ...string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	got := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		got = append(got, fields[0])
	}
	assert.Equal(t, expectedNames, got,
		"body rows must appear in the order Lifecycle.StatusAll returned them")
}

func runningStatus(name string) deploy.Status {
	return deploy.Status{
		Name:           name,
		ContainerID:    "cid-" + name,
		ContainerName:  "decloud-" + name,
		State:          "running",
		LastDeployID:   "20260426-120000-abc123",
		LastDeployedAt: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC),
	}
}

func TestStatus_NoArgs_DelegatesToStatusAllAndPrintsTable(t *testing.T) {
	mock := installMockLifecycle(t)
	bar := deploy.Status{
		Name:           "bar",
		ContainerID:    "cid-bar",
		ContainerName:  "decloud-bar",
		State:          "stopped",
		LastDeployID:   "20260425-110000-def456",
		LastDeployedAt: time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC),
	}
	mock.EXPECT().StatusAll(gomock.Any()).Return([]deploy.Status{bar, runningStatus("foo")}, nil)

	stdout, stderr, err := runStatusNoArgs(t)
	require.NoError(t, err)
	assertHeaderPresent(t, stdout)
	assertRowPresent(t, stdout, "bar", "stopped")
	assertRowPresent(t, stdout, "foo", "running")
	assertBodyRowOrder(t, stdout, "bar", "foo")
	assert.Empty(t, stderr,
		"healthy multi-row output must not emit anything on stderr")
}

func TestStatus_NoArgs_EmptyListPrintsHeaderOnly(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().StatusAll(gomock.Any()).Return(nil, nil)

	stdout, stderr, err := runStatusNoArgs(t)
	require.NoError(t, err)
	assertHeaderPresent(t, stdout)
	bodyLines := strings.Count(strings.TrimRight(stdout, "\n"), "\n")
	assert.Equal(t, 0, bodyLines,
		"zero registered services must print the header line only, with no body rows (grep/awk friendly)")
	assert.Empty(t, stderr)
}

func TestStatus_NoArgs_StatusAllErrorIsReturnedAndMapsToExitInternal(t *testing.T) {
	mock := installMockLifecycle(t)
	mock.EXPECT().StatusAll(gomock.Any()).
		Return(nil, errors.New("registry: reading services dir: permission denied"))

	_, _, err := runStatusNoArgs(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied",
		"host-level StatusAll error must propagate to the operator")
	assert.Equal(t, ExitInternal, ExitCodeFor(err),
		"unwrapped host-level errors fall through to ExitInternal (70)")
}

func TestStatus_NoArgs_RowErrorDetailRoutesToStderrButNotStdout(t *testing.T) {
	mock := installMockLifecycle(t)
	brokenDetail := "loading service: registry: schema_version mismatch: expected 1, got 2"
	mock.EXPECT().StatusAll(gomock.Any()).Return([]deploy.Status{
		runningStatus("good"),
		{Name: "broken-svc", State: "error", ErrorDetail: brokenDetail},
	}, nil)

	stdout, stderr, err := runStatusNoArgs(t)
	require.NoError(t, err,
		"per-service errors are informational; the command itself still exits 0")
	assertRowPresent(t, stdout, "good", "running")
	assertRowPresent(t, stdout, "broken-svc", "error")
	assert.NotContains(t, stdout, "schema_version mismatch",
		"error detail must NOT bleed into the column-aligned stdout (five-column shape is the contract)")
	assert.Contains(t, stderr, "broken-svc",
		"stderr companion must name the failing service so it is greppable")
	assert.Contains(t, stderr, "schema_version mismatch",
		"stderr companion must carry the wrapped error text for operator triage")
}

func TestStatus_TooManyArgsReturnsUsageError(t *testing.T) {
	installMockLifecycle(t)

	_, _, err := runRoot(t, "status", "foo", "bar")
	require.Error(t, err)
	assert.Equal(t, ExitUsageError, ExitCodeFor(err),
		"MaximumNArgs(1) violation must still route through isCobraUsageError to exit 2")
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
