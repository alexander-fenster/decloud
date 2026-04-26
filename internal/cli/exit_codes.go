package cli

import (
	"errors"
	"strings"

	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/envcap"
	"github.com/alexander-fenster/decloud/internal/registry"
)

const (
	ExitOK              = 0
	ExitUsageError      = 2
	ExitConfigError     = 10
	ExitEnvCaptureFail  = 20
	ExitBuildFail       = 30
	ExitRunFail         = 40
	ExitReadinessFail   = 50
	ExitCaddyReloadFail = 60
	ExitInternal        = 70
)

// errUsage is the package-internal sentinel for misuse of the CLI itself
// (missing-required-flag combinations Cobra cannot enforce declaratively).
var errUsage = errors.New("usage error")

// ExitCodeFor maps an error returned by a Cobra RunE function to the process
// exit code. The mapping is by typed sentinel (errors.Is) rather than string
// matching, so wrapped errors are recognized.
func ExitCodeFor(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, errUsage):
		return ExitUsageError
	case errors.Is(err, registry.ErrMountsNotSupported),
		errors.Is(err, registry.ErrInvalidStrategy),
		errors.Is(err, registry.ErrSchemaMismatch),
		errors.Is(err, registry.ErrUnknownField),
		errors.Is(err, registry.ErrPermissionMode),
		errors.Is(err, registry.ErrSecretsMissing),
		errors.Is(err, registry.ErrNotFound),
		errors.Is(err, envcap.ErrEnvScriptMissing),
		errors.Is(err, envcap.ErrEnvScriptUnreadable):
		return ExitConfigError
	case errors.Is(err, deploy.ErrEnvCapture):
		return ExitEnvCaptureFail
	case errors.Is(err, deploy.ErrBuild):
		return ExitBuildFail
	case errors.Is(err, deploy.ErrRun):
		return ExitRunFail
	case errors.Is(err, deploy.ErrReadiness):
		return ExitReadinessFail
	case errors.Is(err, deploy.ErrCaddyReload):
		return ExitCaddyReloadFail
	default:
		if isCobraUsageError(err) {
			return ExitUsageError
		}
		return ExitInternal
	}
}

// isCobraUsageError detects Cobra pre-run validation errors that indicate
// misuse of the CLI (missing required flags, unknown subcommands, etc.).
// Cobra returns these as plain errors without typed sentinels, so we
// fall back to substring matching.
func isCobraUsageError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "required flag") ||
		strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "accepts")
}
