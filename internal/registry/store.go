package registry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/pelletier/go-toml/v2"
)

//go:generate mockgen -source=store.go -destination=mocks/mock_store.go -package=mocks

// Store is the persistence layer for service registrations.
type Store interface {
	Load(ctx context.Context, name string) (*Service, error)
	Save(ctx context.Context, svc *Service) error
	DeleteOrphanConfig(ctx context.Context, name string) error
	List(ctx context.Context) ([]*Service, error)
	Delete(ctx context.Context, name string) error
}

// NewFSStore returns the filesystem-backed Store implementation.
func NewFSStore(paths config.Paths) Store {
	return &fsStore{paths: paths}
}

type fsStore struct {
	paths config.Paths
}

func (s *fsStore) configPath(name string) string {
	return filepath.Join(s.paths.ServicesDir, name+".toml")
}

func (s *fsStore) secretsDir(name string) string {
	return filepath.Join(s.paths.SecretsDir, name)
}

func (s *fsStore) secretsPath(name string) string {
	return filepath.Join(s.secretsDir(name), "env.toml")
}

func (s *fsStore) Load(ctx context.Context, name string) (*Service, error) {
	cfgPath := s.configPath(name)
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil, fmt.Errorf("registry: reading config %s: %w", cfgPath, err)
	}
	cfg, err := decodeConfig(cfgBytes)
	if err != nil {
		return nil, fmt.Errorf("registry: decoding config %s: %w", cfgPath, err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: config %s has schema_version=%d, expected %d",
			ErrSchemaMismatch, cfgPath, cfg.SchemaVersion, CurrentSchemaVersion)
	}
	if len(cfg.Run.Mounts) > 0 {
		return nil, fmt.Errorf("%w: service %q declares %d mount(s) in %s; mounts are not supported until M2",
			ErrMountsNotSupported, cfg.Name, len(cfg.Run.Mounts), cfgPath)
	}
	if cfg.Strategy != "" && cfg.Strategy != "recreate" {
		return nil, fmt.Errorf("%w: service %q declares strategy=%q in %s; only \"recreate\" is supported in M1",
			ErrInvalidStrategy, cfg.Name, cfg.Strategy, cfgPath)
	}
	cfg.Name = name

	secDir := s.secretsDir(name)
	secPath := s.secretsPath(name)
	dirInfo, dirErr := os.Stat(secDir)
	if dirErr != nil {
		if errors.Is(dirErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSecretsMissing, secPath)
		}
		return nil, fmt.Errorf("registry: stat secrets dir %s: %w", secDir, dirErr)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("%w: secrets dir %s has mode %#o, expected 0700",
			ErrPermissionMode, secDir, dirInfo.Mode().Perm())
	}

	secInfo, secErr := os.Stat(secPath)
	if secErr != nil {
		if errors.Is(secErr, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSecretsMissing, secPath)
		}
		return nil, fmt.Errorf("registry: stat secrets file %s: %w", secPath, secErr)
	}
	if secInfo.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("%w: secrets file %s has mode %#o, expected 0600",
			ErrPermissionMode, secPath, secInfo.Mode().Perm())
	}

	secBytes, err := os.ReadFile(secPath)
	if err != nil {
		return nil, fmt.Errorf("registry: reading secrets %s: %w", secPath, err)
	}
	sec, err := decodeSecrets(secBytes)
	if err != nil {
		return nil, fmt.Errorf("registry: decoding secrets %s: %w", secPath, err)
	}
	if sec.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: secrets %s has schema_version=%d, expected %d",
			ErrSchemaMismatch, secPath, sec.SchemaVersion, CurrentSchemaVersion)
	}
	return &Service{Config: cfg, Secrets: sec}, nil
}

func (s *fsStore) Save(ctx context.Context, svc *Service) error {
	if err := s.validateForSave(svc); err != nil {
		return err
	}
	if err := os.MkdirAll(s.paths.ServicesDir, 0o755); err != nil {
		return fmt.Errorf("registry: mkdir services dir: %w", err)
	}
	cfgBytes, err := marshalTOML(svc.Config)
	if err != nil {
		return fmt.Errorf("registry: marshalling config: %w", err)
	}
	secBytes, err := marshalTOML(svc.Secrets)
	if err != nil {
		return fmt.Errorf("registry: marshalling secrets: %w", err)
	}
	cfgPath := s.configPath(svc.Config.Name)
	if err := writeAtomic(cfgPath, 0o644, cfgBytes); err != nil {
		return fmt.Errorf("registry: writing config: %w", err)
	}
	secDir := s.secretsDir(svc.Config.Name)
	if err := os.MkdirAll(secDir, 0o700); err != nil {
		return fmt.Errorf("%w: mkdir secrets dir %s: %w", ErrPartialWrite, secDir, err)
	}
	if err := os.Chmod(secDir, 0o700); err != nil {
		return fmt.Errorf("%w: chmod secrets dir %s: %w", ErrPartialWrite, secDir, err)
	}
	secPath := s.secretsPath(svc.Config.Name)
	if err := writeAtomic(secPath, 0o600, secBytes); err != nil {
		return fmt.Errorf("%w: writing secrets at %s: %w", ErrPartialWrite, secPath, err)
	}
	return nil
}

func (s *fsStore) Delete(ctx context.Context, name string) error {
	secPath := s.secretsPath(name)
	secDir := s.secretsDir(name)
	if err := os.Remove(secPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("registry: removing secrets file %s: %w", secPath, err)
	}
	if err := os.Remove(secDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("registry: removing secrets dir %s: %w", secDir, err)
	}
	cfgPath := s.configPath(name)
	if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("registry: removing config file %s: %w", cfgPath, err)
	}
	return nil
}

func (s *fsStore) DeleteOrphanConfig(ctx context.Context, name string) error {
	cfgPath := s.configPath(name)
	if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *fsStore) List(ctx context.Context) ([]*Service, error) {
	entries, err := os.ReadDir(s.paths.ServicesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("registry: reading services dir %s: %w", s.paths.ServicesDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".toml") || strings.HasSuffix(n, ".tmp") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, ".toml"))
	}
	sort.Strings(names)
	out := make([]*Service, 0, len(names))
	for _, name := range names {
		svc, err := s.Load(ctx, name)
		if err != nil {
			continue
		}
		out = append(out, svc)
	}
	return out, nil
}

func (s *fsStore) validateForSave(svc *Service) error {
	if svc == nil {
		return errors.New("registry: nil service")
	}
	if svc.Config.Name == "" {
		return errors.New("registry: empty service name")
	}
	if svc.Config.SchemaVersion == 0 {
		svc.Config.SchemaVersion = CurrentSchemaVersion
	}
	if svc.Secrets.SchemaVersion == 0 {
		svc.Secrets.SchemaVersion = CurrentSchemaVersion
	}
	if svc.Secrets.Name == "" {
		svc.Secrets.Name = svc.Config.Name
	}
	if svc.Config.LastDeployedAt.IsZero() {
		svc.Config.LastDeployedAt = time.Now().UTC()
	}
	return nil
}

func decodeConfig(data []byte) (ServiceConfig, error) {
	var cfg ServiceConfig
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		var sfe *toml.StrictMissingError
		if errors.As(err, &sfe) {
			return ServiceConfig{}, fmt.Errorf("%w: %s", ErrUnknownField, sfe.String())
		}
		return ServiceConfig{}, err
	}
	return cfg, nil
}

func decodeSecrets(data []byte) (ServiceSecrets, error) {
	var sec ServiceSecrets
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sec); err != nil {
		var sfe *toml.StrictMissingError
		if errors.As(err, &sfe) {
			return ServiceSecrets{}, fmt.Errorf("%w: %s", ErrUnknownField, sfe.String())
		}
		return ServiceSecrets{}, fmt.Errorf("%w: %w", ErrUnknownField, err)
	}
	return sec, nil
}

func marshalTOML(v any) ([]byte, error) {
	return toml.Marshal(v)
}

func writeAtomic(path string, mode os.FileMode, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
