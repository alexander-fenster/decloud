package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCaddyDown_DelegatesToManager(t *testing.T) {
	mock := installMockCaddyManager(t)
	mock.EXPECT().Down(gomock.Any()).Return(nil)

	_, _, err := runRoot(t, "caddy", "down")
	require.NoError(t, err)
}

func TestCaddyDown_ManagerErrorReturnsExitRunFail(t *testing.T) {
	mock := installMockCaddyManager(t)
	mock.EXPECT().Down(gomock.Any()).Return(fmt.Errorf("%w: docker daemon down", caddy.ErrCaddyDown))

	_, _, err := runRoot(t, "caddy", "down")
	require.Error(t, err)
	assert.True(t, errors.Is(err, caddy.ErrCaddyDown))
	assert.Equal(t, ExitRunFail, ExitCodeFor(err))
}

func TestCaddyDown_NoFlags(t *testing.T) {
	installMockCaddyManager(t)

	_, _, err := runRoot(t, "caddy", "down", "--remove-volumes")
	require.Error(t, err)
	assert.Equal(t, ExitUsageError, ExitCodeFor(err))
}
