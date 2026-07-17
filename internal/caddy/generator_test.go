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
	assert.NotContains(t, body, "encode", "encode is site-level; an empty registry has no site blocks")
}

func TestGenerator_EmitsCompressionByDefault(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "foo.example.com"),
	})
	body := readFile(t, out)
	assert.Contains(t, body, "\n    encode zstd gzip\n",
		"compression is on by default, indented 4 spaces like reverse_proxy")
}

func TestGenerator_CompressionPrecedesReverseProxy(t *testing.T) {
	out := generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "foo.example.com"),
	})
	body := readFile(t, out)

	encodeIdx := strings.Index(body, "encode zstd gzip")
	proxyIdx := strings.Index(body, "reverse_proxy")
	require.GreaterOrEqual(t, encodeIdx, 0)
	require.GreaterOrEqual(t, proxyIdx, 0)
	// File order is cosmetic, not semantic: Caddy sorts directives into its own
	// hard-coded order regardless of how they are written. This pins the file's
	// stability and readability (it matches Caddy's canonical order anyway).
	assert.Less(t, encodeIdx, proxyIdx, "encode must be written above reverse_proxy in the site block")
}

func TestGenerator_DisableCompressionOmitsEncode(t *testing.T) {
	streamy := makeService("streamy", 8080, "streamy.example.com")
	streamy.Config.DisableCompression = true

	body := readFile(t, generateToTemp(t, []*registry.Service{streamy}))

	assert.NotContains(t, body, "encode", "opted-out service must carry no encode directive")
	assert.Contains(t, body, "reverse_proxy decloud-streamy:8080",
		"only the encode line is omitted; the rest of the site block must survive")
}

func TestGenerator_MixedCompressionSettings(t *testing.T) {
	zeta := makeService("zeta", 9000, "zeta.example.com")
	zeta.Config.DisableCompression = true

	body := readFile(t, generateToTemp(t, []*registry.Service{
		makeService("alpha", 8000, "alpha.example.com"),
		zeta,
	}))

	assert.Equal(t, 1, strings.Count(body, "encode zstd gzip"),
		"one hostname each, so exactly one service carries encode")
	alphaIdx := strings.Index(body, "alpha.example.com {")
	encodeIdx := strings.Index(body, "encode zstd gzip")
	zetaIdx := strings.Index(body, "zeta.example.com {")
	require.GreaterOrEqual(t, alphaIdx, 0)
	require.GreaterOrEqual(t, encodeIdx, 0)
	require.GreaterOrEqual(t, zetaIdx, 0)
	assert.Less(t, alphaIdx, encodeIdx, "the encode line must sit inside alpha's block")
	assert.Less(t, encodeIdx, zetaIdx, "the encode line must not leak into zeta's block")
}

func TestGenerator_MultiHostnameServiceEncodesEveryBlock(t *testing.T) {
	body := readFile(t, generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "one.example.com", "two.example.com"),
	}))
	assert.Equal(t, 2, strings.Count(body, "encode zstd gzip"),
		"each hostname is its own site block and each needs its own encode")

	streamy := makeService("streamy", 8080, "three.example.com", "four.example.com")
	streamy.Config.DisableCompression = true
	streamyBody := readFile(t, generateToTemp(t, []*registry.Service{streamy}))
	assert.Equal(t, 0, strings.Count(streamyBody, "encode zstd gzip"),
		"opting out must clear every one of the service's blocks")
}

func TestGenerator_EncodeIsNotInGlobalOptionsBlock(t *testing.T) {
	body := readFile(t, generateToTemp(t, []*registry.Service{
		makeService("foo", 8080, "foo.example.com"),
	}))

	globalEnd := strings.Index(body, "\n}\n")
	require.Greater(t, globalEnd, 0)
	assert.NotContains(t, body[:globalEnd], "encode",
		"encode is a site-level directive; in the global options block it fails caddy validate")
}
