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

// docker inspect decloud-foo --format '{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}'
func TestCLIDriver_InspectArgsAndParse(t *testing.T) {
	var records []recordedCmd
	d := driverWith(scriptedFactory(&records,
		`echo '{"id":"cid12345","state":"running","labels":null}'`))

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

// docker inspect decloud-foo --format '{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}'
// Labels JSON-emitted so values containing whitespace/=/quotes parse safely (v2 §3.5.1).
func TestCLIDriver_InspectParsesDecloudServiceLabel(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo '{"id":"cid12345","state":"running","labels":{"decloud.service":"foo"}}'`))

	res, err := d.Inspect(context.Background(), "decloud-foo")
	require.NoError(t, err)
	assert.Equal(t, "cid12345", res.ContainerID)
	assert.Equal(t, "running", res.State)
	assert.Equal(t, "foo", res.Labels["decloud.service"],
		"Labels map must surface decloud.service from docker JSON output")
}

func TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo '{"id":"cid12345","state":"running","labels":null}'`))

	res, err := d.Inspect(context.Background(), "decloud-foo")
	require.NoError(t, err)
	assert.Equal(t, "running", res.State)
	assert.Nil(t, res.Labels,
		"Labels must be nil when docker reports no labels on the container")
}

// docker pull caddy:2
func TestCLIDriver_ImagePullArgs(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.ImagePull(context.Background(), "caddy:2"))
	require.Len(t, records, 1)
	assert.Equal(t, "docker", records[0].Name)
	assert.Equal(t, []string{"pull", "caddy:2"}, records[0].Args)
}

func TestCLIDriver_ImagePullPropagatesStderrOnFailure(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "manifest unknown" 1>&2; exit 1`))
	err := d.ImagePull(context.Background(), "caddy:nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest unknown")
}

// docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp
func TestCLIDriver_ExecArgsBasic(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	require.NoError(t, d.Exec(context.Background(), ExecOptions{
		Container: "decloud-caddy",
		Cmd:       []string{"caddy", "validate", "--config", "/etc/caddy/Caddyfile.tmp"},
	}))
	require.Len(t, records, 1)
	assert.Equal(t, "docker", records[0].Name)
	assert.Equal(t, []string{
		"exec", "decloud-caddy",
		"caddy", "validate", "--config", "/etc/caddy/Caddyfile.tmp",
	}, records[0].Args)
}

func TestCLIDriver_ExecPropagatesContainerNotFound(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "Error: No such container: decloud-caddy" 1>&2; exit 1`))
	err := d.Exec(context.Background(), ExecOptions{
		Container: "decloud-caddy",
		Cmd:       []string{"caddy", "reload"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrContainerNotFound))
}

func TestCLIDriver_ExecPropagatesGenericError(t *testing.T) {
	d := driverWith(scriptedFactory(&[]recordedCmd{},
		`echo "validate: bad caddyfile" 1>&2; exit 1`))
	err := d.Exec(context.Background(), ExecOptions{
		Container: "decloud-caddy",
		Cmd:       []string{"caddy", "validate"},
	})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrContainerNotFound))
	assert.Contains(t, err.Error(), "bad caddyfile")
}

// docker run -d --name decloud-caddy --network decloud --restart unless-stopped \
//
//	--label decloud.managed=caddy \
//	-p 0.0.0.0:80:80/tcp -p [::]:80:80/tcp \
//	-p 0.0.0.0:443:443/tcp -p [::]:443:443/tcp \
//	-p 0.0.0.0:443:443/udp -p [::]:443:443/udp \
//	-v /opt/decloud/config/caddy:/etc/caddy:ro \
//	-v decloud_caddy_data:/data \
//	-v decloud_caddy_config:/config \
//	caddy:2
func TestCLIDriver_RunWithOptionsCaddyShape(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), caddyRunOptionsFixture())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, []string{
		"run", "-d",
		"--name", "decloud-caddy",
		"--network", "decloud",
		"--restart", "unless-stopped",
		"--label", "decloud.managed=caddy",
		"-p", "0.0.0.0:80:80/tcp",
		"-p", "[::]:80:80/tcp",
		"-p", "0.0.0.0:443:443/tcp",
		"-p", "[::]:443:443/tcp",
		"-p", "0.0.0.0:443:443/udp",
		"-p", "[::]:443:443/udp",
		"-v", "/opt/decloud/config/caddy:/etc/caddy:ro",
		"-v", "decloud_caddy_data:/data",
		"-v", "decloud_caddy_config:/config",
		"caddy:2",
	}, records[0].Args)
}

func TestCLIDriver_RunWithOptionsDualStackPorts(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name:    "x",
		Image:   "img",
		Network: "decloud",
		Restart: "no",
		Ports: []PortMap{
			{HostBind: "0.0.0.0", HostPort: 1234, ContainerPort: 5678, Proto: "tcp"},
			{HostBind: "[::]", HostPort: 1234, ContainerPort: 5678, Proto: "tcp"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, portFlagsFromArgs(records[0].Args),
		[]string{"0.0.0.0:1234:5678/tcp", "[::]:1234:5678/tcp"})
}

// docker run -d --name decloud-foo --network decloud --restart unless-stopped \
//
//	--label decloud.service=foo -v /host:/dst:ro -v vol:/dst decloud-foo:abc123
func TestCLIDriver_RunPassesVolumeFlags(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.Run(context.Background(), RunRequest{
		Name:    "decloud-foo",
		Image:   "decloud-foo:abc123",
		Network: "decloud",
		Restart: "unless-stopped",
		Volumes: []VolumeMount{
			{Source: "/host", Target: "/dst", ReadOnly: true, IsNamed: false},
			{Source: "vol", Target: "/dst2", ReadOnly: false, IsNamed: true},
		},
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, []string{"/host:/dst:ro", "vol:/dst2"}, volumeFlagsFromArgs(records[0].Args))
}

func TestCLIDriver_RunWithOptionsBindReadOnly(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Volumes: []VolumeMount{
			{Source: "/host", Target: "/dst", ReadOnly: true, IsNamed: false},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, volumeFlagsFromArgs(records[0].Args), []string{"/host:/dst:ro"})
}

func TestCLIDriver_RunWithOptionsNamedVolumeNotReadOnly(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Volumes: []VolumeMount{
			{Source: "vol", Target: "/dst", ReadOnly: false, IsNamed: true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, volumeFlagsFromArgs(records[0].Args), []string{"vol:/dst"})
}

func TestCLIDriver_RunWithOptionsLabelsSorted(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Labels: map[string]string{"b": "2", "a": "1"},
	})
	require.NoError(t, err)
	assert.Equal(t, labelFlagsFromArgs(records[0].Args), []string{"a=1", "b=2"})
}

func TestCLIDriver_RunWithOptionsPortsDeclaredOrder(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Ports: []PortMap{
			{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
			{HostBind: "0.0.0.0", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, portFlagsFromArgs(records[0].Args),
		[]string{"0.0.0.0:443:443/tcp", "0.0.0.0:80:80/tcp"})
}

func TestCLIDriver_RunWithOptionsPortDefaultProto(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Ports: []PortMap{
			{HostBind: "0.0.0.0", HostPort: 80, ContainerPort: 80},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, portFlagsFromArgs(records[0].Args), []string{"0.0.0.0:80:80/tcp"})
}

func TestCLIDriver_RunWithOptionsEmptyHostBind(t *testing.T) {
	var records []recordedCmd
	d := driverWith(recordingFactory(&records))

	_, err := d.RunWithOptions(context.Background(), RunOptions{
		Name: "x", Image: "img", Network: "n", Restart: "no",
		Ports: []PortMap{
			{HostBind: "", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, portFlagsFromArgs(records[0].Args), []string{"80:80/tcp"})
}

func TestFormatPortMap_DoesNotAutoBracketIPv6(t *testing.T) {
	assert.Equal(t, "[::]:80:80/tcp", formatPortMap(PortMap{
		HostBind: "[::]", HostPort: 80, ContainerPort: 80, Proto: "tcp",
	}))
	assert.Equal(t, "0.0.0.0:80:80/tcp", formatPortMap(PortMap{
		HostBind: "0.0.0.0", HostPort: 80, ContainerPort: 80, Proto: "tcp",
	}))
}

func TestFormatPortMap_EmptyHostBindOmitsBindSegment(t *testing.T) {
	assert.Equal(t, "80:80/tcp", formatPortMap(PortMap{
		HostPort: 80, ContainerPort: 80, Proto: "tcp",
	}))
}

func caddyRunOptionsFixture() RunOptions {
	return RunOptions{
		Name:    "decloud-caddy",
		Image:   "caddy:2",
		Network: "decloud",
		Restart: "unless-stopped",
		Labels:  map[string]string{"decloud.managed": "caddy"},
		Ports: []PortMap{
			{HostBind: "0.0.0.0", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
			{HostBind: "[::]", HostPort: 80, ContainerPort: 80, Proto: "tcp"},
			{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
			{HostBind: "[::]", HostPort: 443, ContainerPort: 443, Proto: "tcp"},
			{HostBind: "0.0.0.0", HostPort: 443, ContainerPort: 443, Proto: "udp"},
			{HostBind: "[::]", HostPort: 443, ContainerPort: 443, Proto: "udp"},
		},
		Volumes: []VolumeMount{
			{Source: "/opt/decloud/config/caddy", Target: "/etc/caddy", ReadOnly: true, IsNamed: false},
			{Source: "decloud_caddy_data", Target: "/data", ReadOnly: false, IsNamed: true},
			{Source: "decloud_caddy_config", Target: "/config", ReadOnly: false, IsNamed: true},
		},
	}
}

func portFlagsFromArgs(args []string) []string {
	return flagValuesByName(args, "-p")
}

func volumeFlagsFromArgs(args []string) []string {
	return flagValuesByName(args, "-v")
}

func labelFlagsFromArgs(args []string) []string {
	return flagValuesByName(args, "--label")
}

func flagValuesByName(args []string, flag string) []string {
	out := []string{}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			out = append(out, args[i+1])
		}
	}
	return out
}
