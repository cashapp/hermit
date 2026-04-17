package envars

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/repr"
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
			assert.Equal(t, test.expected, actual)
			tr = transform("", actual)
			test.op.Revert(tr)
			assert.Equal(t, test.env, tr.Combined())
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
		&Set{Name: "NPM_CONFIG_PREFIX", Value: "/node_modules"},
		&Set{Name: "GOPATH", Value: "/home/larry/go"},
		&Prepend{Name: "PATH", Value: "/usr/bin"},
		&Set{Name: "GOPATH", Value: "/home/moe/go"},
		&Unset{Name: "GOPATH"},
		&Prepend{Name: "PATH", Value: "${NPM_CONFIG_PREFIX}/bin"},
		&Prepend{Name: "PATH", Value: "/usr/local/bin"},
		&Set{Name: "HERMIT_BIN", Value: "${GOBIN}/bin"},
	}
	expected := Envars{
		"GOBIN":                               "/go/bin",
		"HERMIT_BIN":                          "/go/bin/bin",
		"NPM_CONFIG_PREFIX":                   "/node_modules",
		"PATH":                                "/usr/local/bin:/node_modules/bin:/usr/bin:/bin",
		"_HERMIT_OLD_GOPATH_A3751075A9D52FD8": "/home/moe/go",
		"_HERMIT_OLD_GOPATH_D3B9A60664850146": "/go",
		"_HERMIT_OLD_GOPATH_1B15BBB670152CB3": "/home/larry/go",
	}
	actual := original.Apply("", ops).Combined()
	assert.Equal(t, expected, actual)
	actual = actual.Revert("", ops).Combined()
	assert.Equal(t, original, actual)
}

func TestTransform(t *testing.T) {
	tr := transform("", Envars{
		"PATH": "/bin",
	})
	tr.set("GOPATH", "/go/bin")
	tr.set("PATH", "/usr/bin:${PATH}")
	assert.Equal(t, Envars{"PATH": "/usr/bin:/bin", "GOPATH": "/go/bin"}, tr.Combined())
	assert.Equal(t, Envars{"GOPATH": "/go/bin", "PATH": "/usr/bin:/bin"}, tr.Changed(false))
}

func TestIssue47(t *testing.T) {
	original := Envars{
		"PATH":       "/bin",
		"HERMIT_ENV": "/home/user/project",
	}
	pkg := Envars{
		"NPM_CONFIG_PREFIX": "${HERMIT_ENV}/.hermit/node",
		"PATH":              "${HERMIT_ENV}/node_modules/.bin:${NPM_CONFIG_PREFIX}/bin:${PATH}",
	}
	ops := Infer(pkg.System())
	actual := original.Apply("/home/user/project", ops).Combined()
	expected := Envars{
		"HERMIT_ENV":        "/home/user/project",
		"NPM_CONFIG_PREFIX": "/home/user/project/.hermit/node",
		"PATH":              "/home/user/project/node_modules/.bin:/home/user/project/.hermit/node/bin:/bin",
	}
	assert.Equal(t, expected, actual)
	reverted := expected.Revert("/home/user/project", ops).Combined()
	assert.Equal(t, original, reverted)
}

func TestAlreadyPrefixed(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		prepend  string
		expected bool
	}{
		{"SingleAtFront", "/usr/bin:/bin", "/usr/bin", true},
		{"SingleNotAtFront", "/bin:/usr/bin", "/usr/bin", false},
		{"MultiAtFront", "/a:/b:/bin", "/a:/b", true},
		{"MultiNotAtFront", "/bin:/a:/b", "/a:/b", false},
		{"EmptyValue", "", "/usr/bin", false},
		{"EmptyPrepend", "/bin", "", false},
		{"ExactMatch", "/usr/bin", "/usr/bin", true},
		{"PrependLongerThanValue", "/a", "/a:/b", false},
		{"PartialPrefixMismatch", "/a:/c:/bin", "/a:/b", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := alreadyPrefixed(test.value, test.prepend)
			assert.Equal(t, test.expected, actual)
		})
	}
}

// Test that Prepend is a no-op when paths are already at the front.
// This simulates hermit exec when the caller (e.g. bloxlet) has already
// activated Hermit and set up PATH with all the correct paths.
func TestPrependNoOpWhenAlreadyAtFront(t *testing.T) {
	// Single-element: PATH already has /hermit/bin at the front
	env := Envars{
		"PATH": "/hermit/bin:/custom:/usr/bin",
	}
	ops := Ops{
		&Prepend{Name: "PATH", Value: "/hermit/bin"},
	}
	actual := env.Apply("", ops).Combined()
	assert.Equal(t, env, actual)

	// Revert should still work correctly
	reverted := actual.Revert("", ops).Combined()
	assert.Equal(t, Envars{"PATH": "/custom:/usr/bin"}, reverted)

	// Multi-element: PATH already has /a:/b at the front
	env2 := Envars{
		"PATH": "/a:/b:/custom:/usr/bin",
	}
	ops2 := Ops{
		&Prepend{Name: "PATH", Value: "/a:/b"},
	}
	actual2 := env2.Apply("", ops2).Combined()
	assert.Equal(t, env2, actual2)

	reverted2 := actual2.Revert("", ops2).Combined()
	assert.Equal(t, Envars{"PATH": "/custom:/usr/bin"}, reverted2)

	// Present but not at front: should still move to front
	env3 := Envars{
		"PATH": "/bin:/usr/bin",
	}
	ops3 := Ops{
		&Prepend{Name: "PATH", Value: "/usr/bin"},
	}
	actual3 := env3.Apply("", ops3).Combined()
	assert.Equal(t, Envars{"PATH": "/usr/bin:/bin"}, actual3)
}

func TestEncodeDecodeOps(t *testing.T) {
	actual := Ops{
		&Append{"APPEND", "${APPEND}:text"},
		&Prepend{"PREPEND", "text:${PREPEND}"},
		&Set{"SET", "text"},
		&Unset{"UNSET"},
		&Force{"FORCE", "text"},
		&Prefix{"PREFIX", "prefix_"},
	}
	data, err := MarshalOps(actual)
	assert.NoError(t, err)
	t.Log(string(data))
	expected, err := UnmarshalOps(data)
	assert.NoError(t, err)
	assert.Equal(t, repr.String(expected, repr.Indent("  ")), repr.String(actual, repr.Indent("  ")))
}
