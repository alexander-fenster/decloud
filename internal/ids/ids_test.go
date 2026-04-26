package ids_test

import (
	"regexp"
	"testing"

	"github.com/alexander-fenster/decloud/internal/ids"
	"github.com/stretchr/testify/assert"
)

var deployIDFormat = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[a-f0-9]{6}$`)

func TestNewDeployID_FormatRegex(t *testing.T) {
	id := ids.NewDeployID()
	assert.Regexp(t, deployIDFormat, id, "deploy id %q must match YYYYMMDD-HHMMSS-XXXXXX hex pattern", id)
}

func TestNewDeployID_UniqueAcrossRapidCalls(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := ids.NewDeployID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q on iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestContainerName_M1Format(t *testing.T) {
	assert.Equal(t, "decloud-foo", ids.ContainerName("foo"))
	assert.Equal(t, "decloud-my-app", ids.ContainerName("my-app"))
}

func TestImageRef_Format(t *testing.T) {
	assert.Equal(t, "decloud-foo:20260426-120000-abc123", ids.ImageRef("foo", "20260426-120000-abc123"))
}
