package registry

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// volumeNameRE locks Docker's named-volume name format
// ([a-zA-Z0-9][a-zA-Z0-9_.-]+) — first char alphanumeric, two-char minimum.
var volumeNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`)

// IsNamed reports whether m references a Docker named volume (rather than a
// host bind path). The convention: HostPath values starting with "/" are bind
// mounts; all other forms are named-volume references. The TOML field is
// historically named host_path; for named volumes the same field carries the
// volume name. An empty HostPath returns false defensively.
func (m Mount) IsNamed() bool {
	return m.HostPath != "" && !strings.HasPrefix(m.HostPath, "/")
}

// ValidateMount reports whether m is a well-formed Mount. It performs
// grammar-only checks: source non-empty, container_path absolute, named-volume
// regex. It does NOT stat the host path (loader-time stat would punish the
// recoverable state where /mnt/data is not yet automounted after a host
// reboot; see _tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md
// §1 Decision 1).
func ValidateMount(m Mount) error {
	if m.HostPath == "" {
		return fmt.Errorf("source must be non-empty")
	}
	if m.ContainerPath == "" {
		return fmt.Errorf("container_path must be non-empty")
	}
	if !filepath.IsAbs(m.ContainerPath) {
		return fmt.Errorf("container_path must be absolute, got %q", m.ContainerPath)
	}
	if !strings.HasPrefix(m.HostPath, "/") {
		if !volumeNameRE.MatchString(m.HostPath) {
			return fmt.Errorf("named-volume source %q must match [a-zA-Z0-9][a-zA-Z0-9_.-]+", m.HostPath)
		}
	}
	return nil
}

// ValidateMounts validates each entry and additionally rejects duplicate
// container_paths within the slice. Loader callers receive errors wrapped
// with ErrInvalidMount; CLI callers should NOT use this entry point — use
// the per-mount ValidateMount helper plus FindDuplicateTarget directly so
// the CLI wraps with errUsage instead.
func ValidateMounts(mounts []Mount, name, path string) error {
	for i, m := range mounts {
		if err := ValidateMount(m); err != nil {
			return fmt.Errorf("%w: service %q mount[%d] in %s: %w", ErrInvalidMount, name, i, path, err)
		}
	}
	if first, dup, ok := FindDuplicateTarget(mounts); ok {
		return fmt.Errorf("%w: service %q mount[%d] in %s: duplicate container_path %q (also at mount[%d])",
			ErrInvalidMount, name, dup, path, mounts[dup].ContainerPath, first)
	}
	return nil
}

// ParseMountString parses one --mount flag value into a Mount. Format:
//
//	<source>:<container-path>[:ro]
//
// where <source> is either an absolute host path (starts with "/") or a
// Docker named-volume name. Returns the constructed Mount and a non-nil error
// (NOT wrapped with ErrInvalidMount) if the grammar fails.
func ParseMountString(raw string) (Mount, error) {
	if raw == "" {
		return Mount{}, fmt.Errorf("empty")
	}
	parts := strings.Split(raw, ":")
	var src, target string
	var ro bool
	switch len(parts) {
	case 2:
		src, target = parts[0], parts[1]
	case 3:
		src, target = parts[0], parts[1]
		switch parts[2] {
		case "ro":
			ro = true
		default:
			return Mount{}, fmt.Errorf("unsupported mode flag %q (only \"ro\" is supported)", parts[2])
		}
	default:
		return Mount{}, fmt.Errorf("expected <source>:<target>[:ro], got %d component(s)", len(parts))
	}
	m := Mount{HostPath: src, ContainerPath: target, ReadOnly: ro}
	if err := ValidateMount(m); err != nil {
		return Mount{}, err
	}
	return m, nil
}

// FindDuplicateTarget reports the first duplicate container_path in mounts.
// Returns (firstIdx, dupIdx, true) when a duplicate exists, or (0, 0, false)
// when all targets are unique or the slice is empty. The bare-error shape
// lets CLI callers wrap with errUsage without re-routing through
// ErrInvalidMount; internal callers should prefer the validated entry point
// ValidateMounts.
func FindDuplicateTarget(mounts []Mount) (firstIdx, dupIdx int, found bool) {
	seen := make(map[string]int, len(mounts))
	for i, m := range mounts {
		if first, ok := seen[m.ContainerPath]; ok {
			return first, i, true
		}
		seen[m.ContainerPath] = i
	}
	return 0, 0, false
}
