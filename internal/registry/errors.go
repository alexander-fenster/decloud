package registry

import "errors"

var (
	ErrNotFound        = errors.New("registry: service not found")
	ErrSecretsMissing  = errors.New("registry: config exists but secrets file is missing")
	ErrPermissionMode  = errors.New("registry: file or directory has wrong permission mode")
	ErrSchemaMismatch  = errors.New("registry: schema_version mismatch")
	ErrUnknownField    = errors.New("registry: unknown field in TOML")
	ErrInvalidMount    = errors.New("registry: invalid mount")
	ErrInvalidStrategy = errors.New("registry: strategy not supported in M1")
	ErrPartialWrite    = errors.New("registry: partial write (config wrote, secrets failed)")
)
