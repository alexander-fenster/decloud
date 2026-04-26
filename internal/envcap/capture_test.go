package envcap_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexander-fenster/decloud/internal/envcap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "env.sh")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func capture(t *testing.T, body string) (map[string]string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return envcap.New().Capture(ctx, writeScript(t, body))
}

func TestEnvcap_ExportSimple(t *testing.T) {
	got, err := capture(t, `export FOO=bar`+"\n")
	require.NoError(t, err)
	assert.Equal(t, "bar", got["FOO"])
}

func TestEnvcap_UnexportedAssignmentCapturedViaSetA(t *testing.T) {
	got, err := capture(t, `FOO=bar`+"\n")
	require.NoError(t, err)
	assert.Equal(t, "bar", got["FOO"])
}

func TestEnvcap_MultilineValue(t *testing.T) {
	body := "export PEM=\"-----BEGIN-----\nline2\nline3\n-----END-----\"\n"
	got, err := capture(t, body)
	require.NoError(t, err)
	assert.Equal(t, "-----BEGIN-----\nline2\nline3\n-----END-----", got["PEM"])
}

func TestEnvcap_UnicodeValue(t *testing.T) {
	got, err := capture(t, "export GREETING=\"héllo wörld\"\n")
	require.NoError(t, err)
	assert.Equal(t, "héllo wörld", got["GREETING"])
}

func TestEnvcap_ValueWithEqualsSignKeepsRest(t *testing.T) {
	got, err := capture(t, `export KEY="a=b=c"`+"\n")
	require.NoError(t, err)
	assert.Equal(t, "a=b=c", got["KEY"])
}

func TestEnvcap_EmptyValue(t *testing.T) {
	got, err := capture(t, `export EMPTY=`+"\n")
	require.NoError(t, err)
	v, ok := got["EMPTY"]
	require.True(t, ok, "EMPTY should be present even with empty value")
	assert.Equal(t, "", v)
}

func TestEnvcap_ScriptDefinedFunctionNotEmitted(t *testing.T) {
	got, err := capture(t, "myfn() { :; }\nexport KEEP=yes\n")
	require.NoError(t, err)
	assert.Equal(t, "yes", got["KEEP"])
	for k := range got {
		assert.NotContains(t, k, "BASH_FUNC", "bash internals must be filtered: %s", k)
		assert.NotEqual(t, "myfn", k)
	}
}

func TestEnvcap_OperatorEnvDoesNotLeak(t *testing.T) {
	t.Setenv("OPERATOR_SECRET_LEAK", "leaked-value")
	got, err := capture(t, `export OWN_VAR=mine`+"\n")
	require.NoError(t, err)
	_, present := got["OPERATOR_SECRET_LEAK"]
	assert.False(t, present, "operator env must not leak into the captured env")
	assert.Equal(t, "mine", got["OWN_VAR"])
}

func TestEnvcap_BashInternalsExcludedByBaselineDiff(t *testing.T) {
	got, err := capture(t, `export ONLY_MINE=1`+"\n")
	require.NoError(t, err)
	for _, internal := range []string{"PWD", "SHLVL", "BASH_VERSION", "PATH", "HOME"} {
		_, present := got[internal]
		assert.False(t, present, "bash internal %s must be stripped by baseline diff", internal)
	}
}

func TestEnvcap_SetEFalseFailsWithCapturedStderr(t *testing.T) {
	got, err := capture(t, "set -e\nfalse\nexport NEVER=hit\n")
	require.Error(t, err)
	assert.Nil(t, got)
}

func TestEnvcap_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scriptPath := writeScript(t, `sleep 5; export X=1`+"\n")
	_, err := envcap.New().Capture(ctx, scriptPath)
	require.Error(t, err)
}

func TestEnvcap_ScriptDoesNotExist(t *testing.T) {
	_, err := envcap.New().Capture(context.Background(), "/nonexistent/path/to/env.sh")
	require.Error(t, err)
	assert.True(t, errors.Is(err, envcap.ErrEnvScriptMissing))
}

func TestEnvcap_EmptyPathReturnsNilNil(t *testing.T) {
	got, err := envcap.New().Capture(context.Background(), "")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEnvcap_MissingPathReturnsErrEnvScriptMissing(t *testing.T) {
	_, err := envcap.New().Capture(context.Background(), "/nonexistent/path/x")
	require.Error(t, err)
	assert.True(t, errors.Is(err, envcap.ErrEnvScriptMissing))
}

func TestEnvcap_DirectoryPathReturnsErrEnvScriptUnreadable(t *testing.T) {
	_, err := envcap.New().Capture(context.Background(), t.TempDir())
	require.Error(t, err)
	assert.True(t, errors.Is(err, envcap.ErrEnvScriptUnreadable))
}

func TestEnvcap_SetAOff_VariablesDropped(t *testing.T) {
	got, err := capture(t, "export FOO=before\nset +a\nBAR=after\n")
	require.NoError(t, err)
	assert.Equal(t, "before", got["FOO"])
	_, present := got["BAR"]
	assert.False(t, present, "non-exported var after `set +a` must be dropped")
}

func TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured(t *testing.T) {
	got, err := capture(t, "MY_ARR=(a b c)\nexport MY_ARR\n")
	require.NoError(t, err)
	v, present := got["MY_ARR"]
	if present {
		assert.Equal(t, "a", v, "bash 3.2 indirect-expansion of an array yields only [0]")
	}
}

func TestEnvcap_ReadonlyConflict_FailsWithSetE(t *testing.T) {
	got, err := capture(t, "set -e\nreadonly FOO=bar\nFOO=baz\n")
	require.Error(t, err, "set -e + readonly reassignment must surface as error")
	assert.Nil(t, got)
}
