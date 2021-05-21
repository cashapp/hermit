package envars

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpApplyRevert(t *testing.T) {
	tests := []struct {
		name     string
		env      Envars
		op       Op
		expected Envars
	}{
		{"Append",
			Envars{"PATH": "/bin"},
			&Append{Name: "PATH", Value: "/usr/bin"},
			Envars{"PATH": "/bin:/usr/bin"}},
		{"Prepend",
			Envars{"PATH": "/bin"},
			&Prepend{Name: "PATH", Value: "/usr/bin"},
			Envars{"PATH": "/usr/bin:/bin"}},
		{"SetNoOverwrite",
			Envars{"PATH": "/bin"},
			&Set{Name: "GOPATH", Value: "/home/user/go/bin"},
			Envars{"PATH": "/bin", "GOPATH": "/home/user/go/bin"}},
		{"SetOverwrite",
			Envars{"GOPATH": "/go/bin"},
			&Set{Name: "GOPATH", Value: "/home/user/go/bin"},
			Envars{"_HERMIT_OLD_GOPATH_370576067A214FFF": "/go/bin", "GOPATH": "/home/user/go/bin"}},
		{"UnsetNoOverwrite",
			Envars{"PATH": "/bin"},
			&Unset{Name: "GOPATH"},
			Envars{"PATH": "/bin"}},
		{"UnsetOverwrite",
			Envars{"GOPATH": "/go/bin"},
			&Unset{Name: "GOPATH"},
			Envars{"_HERMIT_OLD_GOPATH_A3751075A9D52FD8": "/go/bin"}},
		{"PrependWithVariablePrefix",
			Envars{"GOBIN": "/go/bin", "PATH": "/bin"},
			&Prepend{Name: "PATH", Value: "${GOBIN}"},
			Envars{"GOBIN": "/go/bin", "PATH": "/go/bin:/bin"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tr := transform("", test.env)
			test.op.Apply(tr)
			actual := tr.Combined()
			require.Equal(t, test.expected, actual)
			tr = transform("", actual)
			test.op.Revert(tr)
			require.Equal(t, test.env, tr.Combined())
		})
	}
}

func TestOpsApplyRevert(t *testing.T) {
	original := Envars{
		"PATH":   "/bin",
		"GOPATH": "/go",
		"GOBIN":  "/go/bin",
	}
	ops := Ops{
		&Set{Name: "GOPATH", Value: "/home/larry/go"},
		&Prepend{Name: "PATH", Value: "/usr/bin"},
		&Set{Name: "GOPATH", Value: "/home/moe/go"},
		&Unset{Name: "GOPATH"},
		&Prepend{Name: "PATH", Value: "/usr/local/bin"},
		&Set{Name: "HERMIT_BIN", Value: "${GOBIN}/bin"},
	}
	expected := Envars{
		"GOBIN":                               "/go/bin",
		"HERMIT_BIN":                          "/go/bin/bin",
		"PATH":                                "/usr/local/bin:/usr/bin:/bin",
		"_HERMIT_OLD_GOPATH_A3751075A9D52FD8": "/home/moe/go",
		"_HERMIT_OLD_GOPATH_D3B9A60664850146": "/go",
		"_HERMIT_OLD_GOPATH_1B15BBB670152CB3": "/home/larry/go",
	}
	actual := original.Apply("", ops).Combined()
	require.Equal(t, expected, actual)
	actual = actual.Revert("", ops).Combined()
	require.Equal(t, original, actual)
}

func TestTransform(t *testing.T) {
	tr := transform("", Envars{
		"PATH": "/bin",
	})
	tr.set("GOPATH", "/go/bin")
	tr.set("PATH", "/usr/bin:${PATH}")
	require.Equal(t, Envars{"PATH": "/usr/bin:/bin", "GOPATH": "/go/bin"}, tr.Combined())
	require.Equal(t, Envars{"GOPATH": "/go/bin", "PATH": "/usr/bin:/bin"}, tr.Changed(false))
}
