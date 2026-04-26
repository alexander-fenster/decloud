package config

import (
	"os"
	"path/filepath"
)

const DefaultRoot = "/opt/declouding"

type Paths struct {
	Root          string
	ConfigDir     string
	ServicesDir   string
	JobsDir       string
	CaddyDir      string
	CaddyfilePath string
	SecretsDir    string
	StateDir      string
	DeploysDir    string
	LogsDir       string
	LogFile       string
}

func NewPaths(root string) Paths {
	if root == "" {
		root = DefaultRoot
	}
	return Paths{
		Root:          root,
		ConfigDir:     filepath.Join(root, "config"),
		ServicesDir:   filepath.Join(root, "config", "services"),
		JobsDir:       filepath.Join(root, "config", "jobs"),
		CaddyDir:      filepath.Join(root, "config", "caddy"),
		CaddyfilePath: filepath.Join(root, "config", "caddy", "Caddyfile"),
		SecretsDir:    filepath.Join(root, "secrets"),
		StateDir:      filepath.Join(root, "state"),
		DeploysDir:    filepath.Join(root, "state", "deploys"),
		LogsDir:       filepath.Join(root, "logs"),
		LogFile:       filepath.Join(root, "logs", "decloud.log"),
	}
}

func RootFromEnv() string {
	if v := os.Getenv("DECLOUD_ROOT"); v != "" {
		return v
	}
	return DefaultRoot
}
