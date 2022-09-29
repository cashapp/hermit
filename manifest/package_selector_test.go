package manifest

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSelector_Matches(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		selector Selector
		want     bool
	}{{
		name:     "name selector matches by name",
		source:   "foobar",
		selector: NameSelector("foobar"),
		want:     true,
	}, {
		name:     "name selector discards by name",
		source:   "foobar",
		selector: NameSelector("foo"),
		want:     false,
	}, {
		name:     "prefix selector matches by version prefix",
		source:   "foo-1.2.3",
		selector: PrefixSelector(ParseReference("foo-1.2")),
		want:     true,
	}, {
		name:     "prefix selector discards by version prefix",
		source:   "foo-1.3.3",
		selector: PrefixSelector(ParseReference("foo-1.2")),
		want:     false,
	}, {
		name:     "exact selector matches exact matches",
		source:   "foo-1.3.3",
		selector: ExactSelector(ParseReference("foo-1.3.3")),
		want:     true,
	}, {
		name:     "exact selector matches discards non exact matches",
		source:   "foo-1.3.3",
		selector: ExactSelector(ParseReference("foo-1.3")),
		want:     false,
	}, {
		name:     "glob selector matches matching strings",
		source:   "foo-1.4.3",
		selector: newGlobSelector(t, "foo-*.[4-9].3"),
		want:     true,
	}, {
		name:     "glob selector discards non matches",
		source:   "foo-*.[4-9].3",
		selector: newGlobSelector(t, "foo-1.3.3"),
		want:     false,
	}, {
		name:     "glob selector matches channels",
		source:   "foo@bar",
		selector: newGlobSelector(t, "foo@bar"),
		want:     true,
	}, {
		name:     "glob selector matches packages with a dash in the name",
		source:   "foo-bar-1.2.3",
		selector: newGlobSelector(t, "foo-bar-1.*"),
		want:     true,
	}, {
		name:     "glob selector matches packages with a number in the name",
		source:   "foo2bar-1.2.3",
		selector: newGlobSelector(t, "foo2bar-1.*"),
		want:     true,
	}, {
		name:     "glob selector with just the name matches channels",
		source:   "foo@bar",
		selector: newGlobSelector(t, "foo"),
		want:     true,
	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ParseReference(tt.source)
			if got := tt.selector.Matches(ref); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newGlobSelector(t *testing.T, str string) Selector {
	t.Helper()
	m, err := ParseGlobSelector(str)
	assert.NoError(t, err)
	return m
}
