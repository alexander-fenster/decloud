package caddy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexander-fenster/decloud/internal/caddy"
	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeService(name string, port int, hostnames ...string) *registry.Service {
	routes := make([]registry.Route, len(hostnames))
	for i, h := range hostnames {
		routes[i] = registry.Route{Hostname: h}
	}
	return &registry.Service{
		Config: registry.ServiceConfig{
			Name:   name,
			Routes: routes,
			Run:    registry.RunSpec{Port: port},
			State:  registry.ServiceState{ContainerName: "decloud-" + name},
		},
	}
}

func generateToTemp(t *testing.T, services []*registry.Service) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "Caddyfile")
	require.NoError(t, caddy.NewGenerator().Generate(out, services))
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestGenerator_OneServiceOneHost(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "foo.example.com"),
	})
	body := readFile(t, out)
	assert.Contains(t, body, "foo.example.com")
	assert.Contains(t, body, "reverse_proxy decloud-foo:8080")
}

func TestGenerator_MultiServiceMultiHostSorted(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("zeta", 9000, "zeta.example.com"),
		makeService("alpha", 8000, "alpha2.example.com", "alpha1.example.com"),
	})
	body := readFile(t, out)
	alphaIdx := strings.Index(body, "alpha1.example.com")
	zetaIdx := strings.Index(body, "zeta.example.com")
	require.GreaterOrEqual(t, alphaIdx, 0)
	require.GreaterOrEqual(t, zetaIdx, 0)
	assert.Less(t, alphaIdx, zetaIdx, "services must be sorted by name")
	assert.Less(t,
		strings.Index(body, "alpha1.example.com"),
		strings.Index(body, "alpha2.example.com"),
		"hostnames within a service must be sorted")
}

func TestGenerator_DropsZeroHostnameServices(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("kept", 8080, "kept.example.com"),
		makeService("hidden", 8080),
	})
	body := readFile(t, out)
	assert.Contains(t, body, "kept.example.com")
	assert.NotContains(t, body, "hidden")
	assert.NotContains(t, body, "decloud-hidden")
}

func TestGenerator_DisablesHTTP3(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "foo.example.com"),
	})
	body := readFile(t, out)

	assert.Contains(t, body, "servers {")
	assert.Contains(t, body, "protocols h1 h2\n", "protocols line must advertise exactly h1 h2")
	assert.NotContains(t, body, "protocols h1 h2 h3", "h3 must not be on the protocols line")
	assert.NotContains(t, body, "h1 h2 h3", "no h3 anywhere on the protocols directive")

	assert.Contains(t, body, "\n    servers {\n", "servers block must be indented 4 spaces")
	assert.Contains(t, body, "\n        protocols h1 h2\n", "protocols directive must be indented 8 spaces")

	protoIdx := strings.Index(body, "protocols h1 h2")
	siteIdx := strings.Index(body, "foo.example.com {")
	require.GreaterOrEqual(t, protoIdx, 0)
	require.GreaterOrEqual(t, siteIdx, 0)
	assert.Less(t, protoIdx, siteIdx, "global options block must precede site blocks")
}

func TestGenerator_EmptyInputProducesHeaderAndGlobalBlock(t *testing.T) {
	out := generateToTemp(t, nil)
	body := readFile(t, out)
	assert.NotContains(t, body, "reverse_proxy")
	assert.NotEmpty(t, strings.TrimSpace(body), "output should at least carry a header comment")
	assert.Contains(t, body, "protocols h1 h2", "empty registry still carries the global options block")
}
