// Each test in this file pairs the asserted argv with the hand-typed equivalent
// `docker` command in a comment, per Don §5 and tech-plan v2 §11. If the
// recorded args drift from the comment, the comment is wrong by definition —
// fix the implementation first.

package dockerdrv

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCmd struct {
	Name string
	Args []string
}

// recordingFactory returns a cmdFactory that captures every invocation and
// then runs `true` so the parent code sees a successful exec.
func recordingFactory(records *[]recordedCmd) cmdFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*records = append(*records, recordedCmd{Name: name, Args: args})
		return exec.CommandContext(ctx, "true")
	}
}

// scriptedFactory runs the given /bin/sh -c script every time the factory is
// called. Useful for asserting parser behavior on synthetic stdout/stderr.
// The original args are forwarded as positional ($1, $2, ...) so the script
// can branch on subcommand (e.g., `if [ "$2" = inspect ]`).
func scriptedFactory(records *[]recordedCmd, script string) cmdFactory {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*records = append(*records, recordedCmd{Name: name, Args: args})
		shArgs := append([]string{"-c", script, name}, args...)
		return exec.CommandContext(ctx, "/bin/sh", shArgs...)
	}
}

func driverWith(f cmdFactory) Driver { return newCLIDriverWithFactory(f) }

// docker build -t decloud-foo:abc123 -f Dockerfile /srv/foo
func TestCLIDriver_BuildArgs(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.Build(context.Background(), BuildRequest{
		ImageRef:   "decloud-foo:abc123",
		SourceDir:  "/srv/foo",
		Dockerfile: "Dockerfile",
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "docker", records[0].Name)
	assert.Equal(t, []string{
		"build", "-t", "decloud-foo:abc123",
		"-f", "Dockerfile", "/srv/foo",
	}, records[0].Args)
}

//	docker run -d --name decloud-foo --network decloud --restart unless-stopped \
//	  --env A=1 --env B=2 --env C=3 --label decloud.service=foo decloud-foo:abc123
func TestCLIDriver_RunArgsWithEnvSorted(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.Run(context.Background(), RunRequest{
		Name:    "decloud-foo",
		Image:   "decloud-foo:abc123",
		Network: "decloud",
		Env:     map[string]string{"C": "3", "A": "1", "B": "2"},
		Restart: "unless-stopped",
		Port:    8080,
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, []string{
		"run", "-d",
		"--name", "decloud-foo",
		"--network", "decloud",
		"--restart", "unless-stopped",
		"--env", "A=1",
		"--env", "B=2",
		"--env", "C=3",
		"--label", "decloud.service=foo",
		"decloud-foo:abc123",
	}, records[0].Args)
}

//	docker run -d --name decloud-foo --network decloud --restart unless-stopped \
//	  --label decloud.service=foo decloud-foo:abc123
func TestCLIDriver_RunArgsWithEmptyEnv(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.Run(context.Background(), RunRequest{
		Name:    "decloud-foo",
		Image:   "decloud-foo:abc123",
		Network: "decloud",
		Restart: "unless-stopped",
		Port:    8080,
	})
	require.NoError(t, err)
	for _, arg := range records[0].Args {
		assert.NotEqual(t, "--env", arg, "no --env flags should appear when env map is empty")
	}
}

// docker stop -t 10 decloud-foo
func TestCLIDriver_StopArgs(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Stop(context.Background(), "decloud-foo", 10*time.Second))
	require.Len(t, records, 1)
	assert.Equal(t, []string{"stop", "-t", "10", "decloud-foo"}, records[0].Args)
}

// docker start decloud-foo
func TestCLIDriver_StartArgs(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Start(context.Background(), "decloud-foo"))
	require.Len(t, records, 1)
	assert.Equal(t, []string{"start", "decloud-foo"}, records[0].Args)
}

// docker rm decloud-foo
func TestCLIDriver_RemoveArgs(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Remove(context.Background(), "decloud-foo"))
	require.Len(t, records, 1)
	assert.Equal(t, []string{"rm", "decloud-foo"}, records[0].Args)
}

// docker inspect decloud-foo --format '{{.Id}} {{.State.Status}}'
func TestCLIDriver_InspectArgsAndParse(t *testing.T) {
	var records []recordedCmd
	d := driverWith(scriptedFactory(&records, "echo cid12345 running"))

	res, err := d.Inspect(context.Background(), "decloud-foo")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "inspect", records[0].Args[0])
	assert.Contains(t, records[0].Args, "decloud-foo")
	assert.Equal(t, "cid12345", res.ContainerID)
	assert.Equal(t, "running", res.State)
}

// docker logs --tail 50 decloud-foo
func TestCLIDriver_LogsArgsTailN(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Logs(context.Background(), "decloud-foo", LogsOptions{Tail: 50}))
	require.Len(t, records, 1)
	assert.Contains(t, records[0].Args, "logs")
	assert.Contains(t, records[0].Args, "--tail")
	assert.Contains(t, records[0].Args, "50")
	assert.Contains(t, records[0].Args, "decloud-foo")
}

// docker logs -f decloud-foo
func TestCLIDriver_LogsArgsFollow(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Logs(context.Background(), "decloud-foo", LogsOptions{Follow: true}))
	require.Len(t, records, 1)
	assert.Contains(t, records[0].Args, "-f")
	assert.Contains(t, records[0].Args, "decloud-foo")
}

// docker network inspect decloud  -> exit 1 (absent)
// docker network create decloud  (creates with default bridge driver)
func TestCLIDriver_NetworkEnsureWhenAbsent(t *testing.T) {
	var records []recordedCmd
	d := driverWith(scriptedFactory(&records, "if [ \"$2\" = inspect ]; then exit 1; else exit 0; fi"))

	require.NoError(t, d.NetworkEnsure(context.Background(), "decloud"))
	require.GreaterOrEqual(t, len(records), 2)
	assert.Equal(t, "inspect", records[0].Args[1])
	createCall := records[len(records)-1]
	assert.Contains(t, createCall.Args, "create")
	for _, arg := range createCall.Args {
		assert.NotEqual(t, "--driver", arg,
			"network ensure must NOT pass --driver (default bridge required by readiness probe)")
	}
}

// docker network inspect decloud  -> exit 0 (already present, no-op)
func TestCLIDriver_NetworkEnsureWhenPresent(t *testing.T) {
	var records []recordedCmd
	d := driverWith(scriptedFactory(&records, "exit 0"))

	require.NoError(t, d.NetworkEnsure(context.Background(), "decloud"))
	for _, r := range records {
		assert.NotContains(t, r.Args, "create",
			"network create must NOT be called when network already exists")
	}
}

// docker inspect decloud-foo --format '{{ .NetworkSettings.Networks.decloud.IPAddress }}'
func TestCLIDriver_ContainerIPArgsAndParse(t *testing.T) {
	var records []recordedCmd
	d := driverWith(scriptedFactory(&records, "echo 172.18.0.5"))

	ip, err := d.ContainerIP(context.Background(), "decloud-foo")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "inspect", records[0].Args[0])
	assert.Contains(t, records[0].Args, "decloud-foo")
	joined := bytes.NewBufferString("")
	for _, a := range records[0].Args {
		joined.WriteString(a)
		joined.WriteString(" ")
	}
	assert.Contains(t, joined.String(), "NetworkSettings.Networks.decloud.IPAddress")
	assert.Equal(t, "172.18.0.5", ip)
}

func TestCLIDriver_ContainerIPEmptyReturnsErrNoBridgeIP(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{}, "echo "))
	_, err := d.ContainerIP(context.Background(), "decloud-foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoBridgeIP))
}

func TestCLIDriver_ContainerIPNotFoundReturnsErrContainerNotFound(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "Error: No such object: decloud-foo" 1>&2; exit 1`))
	_, err := d.ContainerIP(context.Background(), "decloud-foo")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContainerNotFound))
}

func TestCLIDriver_StopReturnsErrContainerNotFound(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "Error response from daemon: No such container: decloud-foo" 1>&2; exit 1`))
	err := d.Stop(context.Background(), "decloud-foo", 10*time.Second)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContainerNotFound))
}

func TestCLIDriver_InspectAbsentContainerReturnsAbsentState(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "Error: No such object: decloud-foo" 1>&2; exit 1`))
	res, err := d.Inspect(context.Background(), "decloud-foo")
	require.NoError(t, err)
	assert.Equal(t, "absent", res.State)
	assert.Equal(t, "", res.ContainerID)
}
