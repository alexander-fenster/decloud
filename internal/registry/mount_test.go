package registry_test

import (
	"errors"
	"testing"

	"github.com/alexander-fenster/decloud/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMount_Table(t *testing.T) {
	cases := []struct {
		name      string
		m         registry.Mount
		wantErr   bool
		errSubstr string
	}{
		{"valid_bind_rw", registry.Mount{HostPath: "/host", ContainerPath: "/data", ReadOnly: false}, false, ""},
		{"valid_bind_ro", registry.Mount{HostPath: "/host", ContainerPath: "/data", ReadOnly: true}, false, ""},
		{"valid_named_rw", registry.Mount{HostPath: "mydata", ContainerPath: "/var", ReadOnly: false}, false, ""},
		{"valid_named_ro", registry.Mount{HostPath: "mydata", ContainerPath: "/var", ReadOnly: true}, false, ""},
		{"empty_source", registry.Mount{HostPath: "", ContainerPath: "/data"}, true, "source must be non-empty"},
		{"empty_target", registry.Mount{HostPath: "/host", ContainerPath: ""}, true, "container_path must be non-empty"},
		{"relative_target", registry.Mount{HostPath: "/host", ContainerPath: "data"}, true, "container_path must be absolute"},
		{"named_invalid_chars", registry.Mount{HostPath: "my data", ContainerPath: "/var"}, true, "named-volume source"},
		{"named_starts_with_dash", registry.Mount{HostPath: "-x", ContainerPath: "/var"}, true, "named-volume source"},
		{"named_too_short", registry.Mount{HostPath: "x", ContainerPath: "/var"}, true, "named-volume source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := registry.ValidateMount(tc.m)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}

func TestValidateMounts_DuplicateContainerPath(t *testing.T) {
	mounts := []registry.Mount{
		{HostPath: "/a", ContainerPath: "/x"},
		{HostPath: "/b", ContainerPath: "/x"},
	}
	err := registry.ValidateMounts(mounts, "foo", "/srv/foo.toml")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrInvalidMount))
	assert.Contains(t, err.Error(), "duplicate container_path")
	assert.Contains(t, err.Error(), "mount[0]")
	assert.Contains(t, err.Error(), "mount[1]")
}

func TestValidateMounts_FirstInvalidStops(t *testing.T) {
	mounts := []registry.Mount{
		{HostPath: "/a", ContainerPath: "/x"},
		{HostPath: "/b", ContainerPath: "relative"},
		{HostPath: "/c", ContainerPath: "also-relative"},
	}
	err := registry.ValidateMounts(mounts, "foo", "/srv/foo.toml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mount[1]")
	assert.NotContains(t, err.Error(), "mount[2]")
}

func TestValidateMounts_EmptyAndNilSliceAreNoOp(t *testing.T) {
	assert.NoError(t, registry.ValidateMounts(nil, "foo", "/srv/foo.toml"))
	assert.NoError(t, registry.ValidateMounts([]registry.Mount{}, "foo", "/srv/foo.toml"))
}

func TestValidateMounts_LoaderErrorNamesPathAndService(t *testing.T) {
	mounts := []registry.Mount{
		{HostPath: "/a", ContainerPath: "relative"},
	}
	err := registry.ValidateMounts(mounts, "foo", "/srv/foo.toml")
	require.Error(t, err)
	assert.True(t, errors.Is(err, registry.ErrInvalidMount))
	assert.Contains(t, err.Error(), `service "foo"`)
	assert.Contains(t, err.Error(), "/srv/foo.toml")
}

func TestParseMountString_Table(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		want      registry.Mount
		wantErr   bool
		errSubstr string
	}{
		{"bind_rw", "/host:/data", registry.Mount{HostPath: "/host", ContainerPath: "/data", ReadOnly: false}, false, ""},
		{"bind_ro", "/host:/data:ro", registry.Mount{HostPath: "/host", ContainerPath: "/data", ReadOnly: true}, false, ""},
		{"named_rw", "mydata:/var", registry.Mount{HostPath: "mydata", ContainerPath: "/var", ReadOnly: false}, false, ""},
		{"named_ro", "mydata:/var:ro", registry.Mount{HostPath: "mydata", ContainerPath: "/var", ReadOnly: true}, false, ""},
		{"empty", "", registry.Mount{}, true, "empty"},
		{"single_component", "/host", registry.Mount{}, true, "got 1 component"},
		{"four_components", "/a:/b:ro:extra", registry.Mount{}, true, "got 4 component"},
		{"mode_rw_explicit", "/h:/d:rw", registry.Mount{}, true, `unsupported mode flag "rw"`},
		{"mode_unknown", "/h:/d:zz", registry.Mount{}, true, `unsupported mode flag "zz"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := registry.ParseMountString(tc.raw)
			if !tc.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}

func TestMount_IsNamed(t *testing.T) {
	cases := []struct {
		name string
		m    registry.Mount
		want bool
	}{
		{"bind_path_is_not_named", registry.Mount{HostPath: "/host", ContainerPath: "/data"}, false},
		{"volume_name_is_named", registry.Mount{HostPath: "vol", ContainerPath: "/data"}, true},
		{"empty_source_is_not_named", registry.Mount{HostPath: "", ContainerPath: "/data"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.m.IsNamed())
		})
	}
}

func TestFindDuplicateTarget_Table(t *testing.T) {
	cases := []struct {
		name      string
		mounts    []registry.Mount
		wantFirst int
		wantDup   int
		wantOk    bool
	}{
		{"empty_slice", []registry.Mount{}, 0, 0, false},
		{"single_mount", []registry.Mount{{ContainerPath: "/a"}}, 0, 0, false},
		{"two_distinct", []registry.Mount{{ContainerPath: "/a"}, {ContainerPath: "/b"}}, 0, 0, false},
		{"two_same", []registry.Mount{{ContainerPath: "/a"}, {ContainerPath: "/a"}}, 0, 1, true},
		{"collision_at_0_and_2", []registry.Mount{
			{ContainerPath: "/a"}, {ContainerPath: "/b"}, {ContainerPath: "/a"},
		}, 0, 2, true},
		{"first_collision_wins", []registry.Mount{
			{ContainerPath: "/a"}, {ContainerPath: "/b"}, {ContainerPath: "/b"}, {ContainerPath: "/a"},
		}, 1, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, dup, ok := registry.FindDuplicateTarget(tc.mounts)
			assert.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				assert.Equal(t, tc.wantFirst, first)
				assert.Equal(t, tc.wantDup, dup)
			}
		})
	}
}
