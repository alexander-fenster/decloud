package registry_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfigTOML = `schema_version = 1
name = "foo"
strategy = "recreate"
last_deployed_at = 2026-04-26T12:00:00Z

[source]
dir = "/srv/foo"

[build]
dockerfile = "Dockerfile"
image_ref = "decloud-foo:20260426-120000-abc123"

[run]
network = "decloud"
port = 8080
restart = "unless-stopped"
mounts = []

[[routes]]
hostname = "foo.example.com"

[readiness]
kind = "http"
http_path = "/healthz"
timeout_secs = 60
interval_secs = 2

[state]
last_deploy_id = "20260426-120000-abc123"
built_image_id = "sha256:abcd"
container_id = "cid"
container_name = "decloud-foo"
last_deployed_by = "operator"
`

const validSecretsTOML = `schema_version = 1
name = "foo"

[env]
DATABASE_URL = "postgres://localhost/foo"
`

func newStore(t *testing.T) (registry.Store, config.Paths) {
	t.Helper()
	root := t.TempDir()
	paths := config.NewPaths(root)
	require.NoError(t, os.MkdirAll(paths.ServicesDir, 0o755))
	require.NoError(t, os.MkdirAll(paths.SecretsDir, 0o755))
	return registry.NewFSStore(paths), paths
}

func writeConfigFile(t *testing.T, paths config.Paths, name, body string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(paths.ServicesDir, 0o755))
	p := filepath.Join(paths.ServicesDir, name+".toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func writeSecretsFile(t *testing.T, paths config.Paths, name, body string, dirMode, fileMode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(paths.SecretsDir, name)
	require.NoError(t, os.MkdirAll(dir, dirMode))
	require.NoError(t, os.Chmod(dir, dirMode))
	p := filepath.Join(dir, "env.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), fileMode))
	require.NoError(t, os.Chmod(p, fileMode))
	return p
}

func newServiceFixture() *registry.Service {
	return &registry.Service{
		Config: registry.ServiceConfig{
			SchemaVersion: 1,
			Name:          "foo",
			Source:        registry.SourceSpec{Dir: "/srv/foo"},
			Build:         registry.BuildSpec{Dockerfile: "Dockerfile", ImageRef: "decloud-foo:20260426-120000-abc123"},
			Run:           registry.RunSpec{Network: "decloud", Port: 8080, Restart: "unless-stopped", Mounts: []registry.Mount{}},
			Routes:        []registry.Route{{Hostname: "foo.example.com"}},
			Strategy:      "recreate",
			Readiness:     registry.ReadinessSpec{Kind: "http", HTTPPath: "/healthz", TimeoutSecs: 60, IntervalSecs: 2},
			State: registry.ServiceState{
				LastDeployID:   "20260426-120000-abc123",
				BuiltImageID:   "sha256:abcd",
				ContainerID:    "cid",
				ContainerName:  "decloud-foo",
				LastDeployedBy: "operator",
			},
			LastDeployedAt: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC),
		},
		Secrets: registry.ServiceSecrets{
			SchemaVersion: 1,
			Name:          "foo",
			Env:           map[string]string{"DATABASE_URL": "postgres://localhost/foo"},
		},
	}
}

func TestStore_RoundTripConfigAndSecrets(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	original := newServiceFixture()

	require.NoError(t, store.Save(ctx, original))

	loaded, err := store.Load(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, original.Config.Name, loaded.Config.Name)
	assert.Equal(t, original.Config.SchemaVersion, loaded.Config.SchemaVersion)
	assert.Equal(t, original.Config.Strategy, loaded.Config.Strategy)
	assert.Equal(t, original.Config.Build.ImageRef, loaded.Config.Build.ImageRef)
	assert.Equal(t, original.Config.Run.Port, loaded.Config.Run.Port)
	assert.Equal(t, original.Config.Routes, loaded.Config.Routes)
	assert.Equal(t, original.Secrets.Env, loaded.Secrets.Env)
}

func TestStore_RoundTripsLastDeployedAt(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	svc := newServiceFixture()
	svc.Config.LastDeployedAt = time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	require.NoError(t, store.Save(ctx, svc))

	loaded, err := store.Load(ctx, "foo")
	require.NoError(t, err)
	assert.True(t, svc.Config.LastDeployedAt.Equal(loaded.Config.LastDeployedAt),
		"want %s got %s", svc.Config.LastDeployedAt, loaded.Config.LastDeployedAt)
}

func TestStore_LoadRejectsUnknownConfigField(t *testing.T) {
	store, paths := newStore(t)
	body := validConfigTOML + "\nbogus_extra_field = 42\n"
	writeConfigFile(t, paths, "foo", body)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrUnknownField)
}

func TestStore_LoadRejectsUnknownSecretsField(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)
	writeSecretsFile(t, paths, "foo", validSecretsTOML+"\nbogus_extra_secret = true\n", 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrUnknownField)
}

func TestStore_LoadRejectsConfigSchemaMismatch(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", `schema_version = 2
name = "foo"
strategy = "recreate"
[source]
dir = "/srv/foo"
[build]
dockerfile = "Dockerfile"
image_ref = "decloud-foo:x"
[run]
network = "decloud"
port = 8080
restart = "unless-stopped"
mounts = []
[readiness]
kind = "http"
http_path = "/healthz"
timeout_secs = 60
interval_secs = 2
[state]
last_deploy_id = ""
built_image_id = ""
container_id = ""
container_name = ""
last_deployed_by = ""
`)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrSchemaMismatch)
}

func TestStore_LoadRejectsCrossFileSchemaMismatch(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)
	writeSecretsFile(t, paths, "foo", `schema_version = 2
name = "foo"
[env]
`, 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrSchemaMismatch)
}

func TestStore_LoadRejectsSecretsFileMode0644(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o644)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrPermissionMode)
}

func TestStore_LoadRejectsSecretsDirMode0755(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o755, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrPermissionMode)
}

func TestStore_LoadConfigWithoutSecretsReturnsErrSecretsMissing(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrSecretsMissing)
}

func TestStore_LoadReturnsErrNotFoundWhenConfigAbsent(t *testing.T) {
	store, _ := newStore(t)
	_, err := store.Load(context.Background(), "doesnotexist")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrNotFound)
}

func TestStore_LoadAcceptsEmptyMountsArray(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o600)

	svc, err := store.Load(context.Background(), "foo")
	require.NoError(t, err)
	assert.Empty(t, svc.Config.Run.Mounts)
}

func TestStore_LoadRejectsNonEmptyMounts(t *testing.T) {
	store, paths := newStore(t)
	body := `schema_version = 1
name = "foo"
strategy = "recreate"
[source]
dir = "/srv/foo"
[build]
dockerfile = "Dockerfile"
image_ref = "decloud-foo:x"
[run]
network = "decloud"
port = 8080
restart = "unless-stopped"
[[run.mounts]]
host_path = "/host/data"
container_path = "/data"
read_only = false
[readiness]
kind = "http"
http_path = "/healthz"
timeout_secs = 60
interval_secs = 2
[state]
last_deploy_id = ""
built_image_id = ""
container_id = ""
container_name = ""
last_deployed_by = ""
`
	writeConfigFile(t, paths, "foo", body)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrMountsNotSupported)
}

func TestStore_LoadRejectsInvalidStrategy(t *testing.T) {
	store, paths := newStore(t)
	body := `schema_version = 1
name = "foo"
strategy = "blue_green"
[source]
dir = "/srv/foo"
[build]
dockerfile = "Dockerfile"
image_ref = "decloud-foo:x"
[run]
network = "decloud"
port = 8080
restart = "unless-stopped"
mounts = []
[readiness]
kind = "http"
http_path = "/healthz"
timeout_secs = 60
interval_secs = 2
[state]
last_deploy_id = ""
built_image_id = ""
container_id = ""
container_name = ""
last_deployed_by = ""
`
	writeConfigFile(t, paths, "foo", body)
	writeSecretsFile(t, paths, "foo", validSecretsTOML, 0o700, 0o600)

	_, err := store.Load(context.Background(), "foo")
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrInvalidStrategy)
}

func TestStore_SaveOrderConfigBeforeSecrets(t *testing.T) {
	store, paths := newStore(t)
	svc := newServiceFixture()
	require.NoError(t, store.Save(context.Background(), svc))

	cfgInfo, cfgErr := os.Stat(filepath.Join(paths.ServicesDir, "foo.toml"))
	secInfo, secErr := os.Stat(filepath.Join(paths.SecretsDir, "foo", "env.toml"))
	require.NoError(t, cfgErr)
	require.NoError(t, secErr)
	assert.True(t, !secInfo.ModTime().Before(cfgInfo.ModTime()),
		"secrets must be created at-or-after config; got config=%s secrets=%s",
		cfgInfo.ModTime(), secInfo.ModTime())
}

func TestStore_DeleteOrderSecretsBeforeConfig(t *testing.T) {
	store, paths := newStore(t)
	require.NoError(t, store.Save(context.Background(), newServiceFixture()))

	require.NoError(t, store.Delete(context.Background(), "foo"))
	_, cfgErr := os.Stat(filepath.Join(paths.ServicesDir, "foo.toml"))
	_, secErr := os.Stat(filepath.Join(paths.SecretsDir, "foo", "env.toml"))
	assert.True(t, errors.Is(cfgErr, os.ErrNotExist), "config must be removed: %v", cfgErr)
	assert.True(t, errors.Is(secErr, os.ErrNotExist), "secrets must be removed: %v", secErr)
}

func TestStore_DeleteIsIdempotentWhenAbsent(t *testing.T) {
	store, _ := newStore(t)
	require.NoError(t, store.Delete(context.Background(), "absent"))
}

func TestStore_SaveAtomicityNoTmpLeftBehind(t *testing.T) {
	store, paths := newStore(t)
	require.NoError(t, store.Save(context.Background(), newServiceFixture()))

	entries, err := os.ReadDir(paths.ServicesDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "leftover tmp file: %s", e.Name())
	}
}

func TestStore_SaveReturnsErrPartialWriteOnSecretsFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses dir-mode write enforcement")
	}
	store, paths := newStore(t)
	require.NoError(t, os.MkdirAll(paths.SecretsDir, 0o755))
	require.NoError(t, os.Chmod(paths.SecretsDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(paths.SecretsDir, 0o755) })

	err := store.Save(context.Background(), newServiceFixture())
	require.Error(t, err)
	assert.ErrorIs(t, err, registry.ErrPartialWrite,
		"secrets-write failure after config-write success must surface as ErrPartialWrite")

	cfgPath := filepath.Join(paths.ServicesDir, "foo.toml")
	_, statErr := os.Stat(cfgPath)
	assert.NoError(t, statErr,
		"config file must be on disk so the orchestrator can call DeleteOrphanConfig")
}

func TestStore_DeleteOrphanConfigRemovesConfig(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "foo", validConfigTOML)

	require.NoError(t, store.DeleteOrphanConfig(context.Background(), "foo"))
	_, err := os.Stat(filepath.Join(paths.ServicesDir, "foo.toml"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestStore_DeleteOrphanConfigIsIdempotent(t *testing.T) {
	store, _ := newStore(t)
	require.NoError(t, store.DeleteOrphanConfig(context.Background(), "neverexisted"))
}

func TestStore_ListReturnsAllSavedServices(t *testing.T) {
	store, _ := newStore(t)
	ctx := context.Background()
	a := newServiceFixture()
	b := newServiceFixture()
	b.Config.Name, b.Secrets.Name = "bar", "bar"
	require.NoError(t, store.Save(ctx, a))
	require.NoError(t, store.Save(ctx, b))

	got, err := store.List(ctx)
	require.NoError(t, err)
	names := []string{got[0].Config.Name, got[1].Config.Name}
	assert.ElementsMatch(t, []string{"foo", "bar"}, names)
}

func TestStore_ListSkipsMalformedFiles(t *testing.T) {
	store, paths := newStore(t)
	writeConfigFile(t, paths, "good", validConfigTOML)
	writeSecretsFile(t, paths, "good", validSecretsTOML, 0o700, 0o600)
	writeConfigFile(t, paths, "broken", "this = is = not = valid = toml\n")

	got, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "good", got[0].Config.Name)
}

func TestStore_SaveSetsCorrectFilePermissions(t *testing.T) {
	store, paths := newStore(t)
	require.NoError(t, store.Save(context.Background(), newServiceFixture()))

	cfgInfo, err := os.Stat(filepath.Join(paths.ServicesDir, "foo.toml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), cfgInfo.Mode().Perm())

	secInfo, err := os.Stat(filepath.Join(paths.SecretsDir, "foo", "env.toml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), secInfo.Mode().Perm())

	dirInfo, err := os.Stat(filepath.Join(paths.SecretsDir, "foo"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}
