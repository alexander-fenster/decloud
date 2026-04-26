package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alexander-fenster/decloud/internal/deploy"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestExitCodeFor_AllSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"nil", nil, ExitOK},
		{"usage", errUsage, ExitUsageError},
		{"wrapped-usage", fmt.Errorf("bad: %w", errUsage), ExitUsageError},
		{"mounts", registry.ErrMountsNotSupported, ExitConfigError},
		{"strategy", registry.ErrInvalidStrategy, ExitConfigError},
		{"schema", registry.ErrSchemaMismatch, ExitConfigError},
		{"unknown-field", registry.ErrUnknownField, ExitConfigError},
		{"perm", registry.ErrPermissionMode, ExitConfigError},
		{"secrets-missing", registry.ErrSecretsMissing, ExitConfigError},
		{"not-found", registry.ErrNotFound, ExitConfigError},
		{"envcap", deploy.ErrEnvCapture, ExitEnvCaptureFail},
		{"build", deploy.ErrBuild, ExitBuildFail},
		{"run", deploy.ErrRun, ExitRunFail},
		{"readiness", deploy.ErrReadiness, ExitReadinessFail},
		{"caddy", deploy.ErrCaddyReload, ExitCaddyReloadFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, ExitCodeFor(tc.err))
		})
	}
}

func TestExitCodeFor_UnknownErrorMapsToInternal(t *testing.T) {
	assert.Equal(t, ExitInternal, ExitCodeFor(errors.New("an unrecognised failure")))
}
