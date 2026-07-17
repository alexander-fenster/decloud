package registry

import "time"

const CurrentSchemaVersion = 1

// ServiceConfig is the on-disk non-secret half. Persisted at
// <root>/config/services/<Name>.toml, mode 0644.
type ServiceConfig struct {
	SchemaVersion int    `toml:"schema_version"`
	Name          string `toml:"name"`

	Source   SourceSpec `toml:"source"`
	Build    BuildSpec  `toml:"build"`
	Run      RunSpec    `toml:"run"`
	Routes   []Route    `toml:"routes"`
	Strategy string     `toml:"strategy"`

	// DisableCompression omits Caddy's `encode` directive from this service's
	// site blocks. Absent/false = compression ON (the default). Set it for
	// streaming (headers-then-idle) backends: Caddy installs its encoding
	// responseWriter on the request's Accept-Encoding alone, so a pre-body
	// Flush() is swallowed and an idle-first event stream hangs — see
	// caddyserver/caddy#6293. The `match` sub-directive cannot prevent this;
	// only omitting `encode` can. Backward-compatible TOML addition (cf.
	// LastDeployedAt below): existing files without the key unmarshal to
	// false and gain compression. Bool polarity mirrors Mount.ReadOnly.
	DisableCompression bool `toml:"disable_compression"`

	Readiness ReadinessSpec `toml:"readiness"`

	State ServiceState `toml:"state"`

	// LastDeployedAt is set to time.Now().UTC() at the moment of a successful
	// Save. Tech-plan v2 §15.1 (accepted by Linus): backward-compatible TOML
	// addition; existing files without the field unmarshal to zero-value.
	LastDeployedAt time.Time `toml:"last_deployed_at"`
}

// ServiceSecrets is the on-disk secret half. Persisted at
// <root>/secrets/<Name>/env.toml, mode 0600 in a 0700 dir.
type ServiceSecrets struct {
	SchemaVersion int               `toml:"schema_version"`
	Name          string            `toml:"name"`
	Env           map[string]string `toml:"env"`
}

// Service is the merged in-memory view. Never persisted directly; always
// split into ServiceConfig + ServiceSecrets at write time.
type Service struct {
	Config  ServiceConfig
	Secrets ServiceSecrets
}

type SourceSpec struct {
	Dir string `toml:"dir"`
}

type BuildSpec struct {
	Dockerfile string `toml:"dockerfile"`
	ImageRef   string `toml:"image_ref"`
}

type RunSpec struct {
	Network string  `toml:"network"`
	Port    int     `toml:"port"`
	Restart string  `toml:"restart"`
	Mounts  []Mount `toml:"mounts"`
}

type Mount struct {
	// HostPath is the mount source. For bind mounts it is an absolute host
	// path starting with "/"; for named volumes it is the volume name. The
	// TOML key is historically named host_path. Use Mount.IsNamed() to
	// distinguish at runtime.
	HostPath      string `toml:"host_path"`
	ContainerPath string `toml:"container_path"`
	ReadOnly      bool   `toml:"read_only"`
}

type Route struct {
	Hostname string `toml:"hostname"`
}

type ReadinessSpec struct {
	Kind         string `toml:"kind"`
	HTTPPath     string `toml:"http_path"`
	TimeoutSecs  int    `toml:"timeout_secs"`
	IntervalSecs int    `toml:"interval_secs"`
}

type ServiceState struct {
	LastDeployID   string `toml:"last_deploy_id"`
	BuiltImageID   string `toml:"built_image_id"`
	ContainerID    string `toml:"container_id"`
	ContainerName  string `toml:"container_name"`
	LastDeployedBy string `toml:"last_deployed_by"`
}
