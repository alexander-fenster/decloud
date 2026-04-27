package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alexander-fenster/decloud/internal/caddy"
	caddymocks "github.com/alexander-fenster/decloud/internal/caddy/mocks"
	"github.com/alexander-fenster/decloud/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func installMockCaddyManager(t *testing.T) *caddymocks.MockManager {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := caddymocks.NewMockManager(ctrl)
	prev := caddyManagerFactory
	caddyManagerFactory = func(_ config.Paths) (caddy.Manager, error) { return mock, nil }
	t.Cleanup(func() { caddyManagerFactory = prev })
	return mock
}

func TestCaddyUp_DelegatesToManager(t *testing.T) {
	mock := installMockCaddyManager(t)
	mock.EXPECT().Up(gomock.Any()).Return(nil)

	_, _, err := runRoot(t, "caddy", "up")
	require.NoError(t, err)
}

func TestCaddyUp_ManagerErrorReturnsExitRunFail(t *testing.T) {
	mock := installMockCaddyManager(t)
	mock.EXPECT().Up(gomock.Any()).Return(fmt.Errorf("%w: docker daemon down", caddy.ErrCaddyUp))

	_, _, err := runRoot(t, "caddy", "up")
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyUp))
	assert.Equal(t, ExitRunFail, ExitCodeFor(err))
}

func TestCaddyUp_NoFlags(t *testing.T) {
	installMockCaddyManager(t)

	_, _, err := runRoot(t, "caddy", "up", "--image", "caddy:2.7.6")
	require.Error(t, err)
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}

func TestCaddyUp_PassesContextThrough(t *testing.T) {
	mock := installMockCaddyManager(t)
	var gotCtx context.Context
	mock.EXPECT().Up(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		gotCtx = ctx
		return nil
	})

	_, _, err := runRoot(t, "caddy", "up")
	require.NoError(t, err)
	assert.NotNil(t, gotCtx)
}
