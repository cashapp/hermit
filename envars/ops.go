package envars

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"reflect"
	"sort"
	"strings"

	"github.com/kballard/go-shellquote"

	"github.com/cashapp/hermit/errors"
)

// Envars is a convenience alias
type Envars map[string]string

// Clone envars.
func (e Envars) Clone() Envars {
	out := make(Envars, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// System renders the Envars in the format expected by the system, ie. KEY=VALUE
func (e Envars) System() []string {
	out := make([]string, 0, len(e))
	for k, v := range e {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(out)
	return out
}

// Apply ops to these Envars and return the resulting Transform.
//
// Envars are not modified.
func (e Envars) Apply(envRoot string, ops Ops) *Transform {
	transform := transform(envRoot, e)
	for _, op := range ops {
		op.Apply(transform)
	}
	return transform
}

// Revert creates a Transform that reverts the application of the provided Ops.
//
// Envars are not modified.
func (e Envars) Revert(envRoot string, ops Ops) *Transform {
	transform := transform(envRoot, e)
	for i := len(ops) - 1; i >= 0; i-- {
		ops[i].Revert(transform)
	}
	return transform
}

// Op is an operation on an environment variable.
type Op interface {
	fmt.Stringer
	Envar() string
	// Apply changes to transform.
	//
	// This may also add/remove extra housekeeping variables to support Revert.
	Apply(transform *Transform)
	// Revert the changes made by Apply.
	Revert(transform *Transform)

	sealed()
}

// Ops to apply to a set of environment variables.
type Ops []Op

// {<type>: <object>} - see marshalKeys for the key types
type encodedOp map[string]json.RawMessage

// MarshalOps to JSON.
func MarshalOps(ops Ops) ([]byte, error) {
	encoded := make([]encodedOp, 0, len(ops))
	for _, op := range ops {
		key, ok := marshalKeys[reflect.TypeOf(op)]
		if !ok {
			panic(fmt.Sprintf("unsupported op type %T", op))
		}
		jop, err := json.Marshal(op)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		encoded = append(encoded, encodedOp{key: jop})
	}
	return json.Marshal(encoded)
}

// UnmarshalOps from JSON.
func UnmarshalOps(data []byte) (Ops, error) {
	var encoded []encodedOp
	err := json.Unmarshal(data, &encoded)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ops := make(Ops, 0, len(encoded))
	for _, enc := range encoded {
		var key string
		var encodedOp json.RawMessage
		for key, encodedOp = range enc { //nolint:revive // empty block is intentional
		}
		typ, ok := unmarshalKeys[key]
		if !ok {
			return nil, errors.Errorf("unsupported envar op key %q", key)
		}
		op := reflect.New(typ).Interface().(Op)
		if err = json.Unmarshal(encodedOp, op); err != nil {
			return nil, errors.WithStack(err)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// Infer uses simple heuristics to build a sequence of transformations for environment variables.
//
// Currently this consists of detecting prepend/append to :-separated lists, set and unset.
func Infer(env []string) Ops {
	ops := make(Ops, 0, len(env))
	for _, envar := range env {
		parts := strings.SplitN(envar, "=", 2)
		key := parts[0]
		value := parts[1]
		var op Op
		switch {
		case strings.HasPrefix(value, "${"+key+"}:") || strings.HasPrefix(value, "$"+key+":"): // Append
			insertion := value[strings.Index(value, ":")+1:]
			op = &Append{
				Name:  key,
				Value: insertion,
			}
		case strings.HasSuffix(value, ":${"+key+"}") || strings.HasSuffix(value, ":$"+key): // Prepend
			insertion := value[:strings.LastIndex(value, ":")]
			op = &Prepend{
				Name:  key,
				Value: insertion,
			}

		case value == "":
			op = &Unset{Name: key}

		default:
			op = &Set{Name: key, Value: value}
		}
		ops = append(ops, op)
	}
	return ops
}

// These tables need to be kept in sync.
var (
	marshalKeys = map[reflect.Type]string{
		reflect.TypeOf(&Append{}):  "a",
		reflect.TypeOf(&Prepend{}): "p",
		reflect.TypeOf(&Set{}):     "s",
		reflect.TypeOf(&Unset{}):   "u",
		reflect.TypeOf(&Force{}):   "f",
		reflect.TypeOf(&Prefix{}):  "P",
	}
	unmarshalKeys = func() map[string]reflect.Type {
		out := make(map[string]reflect.Type, len(marshalKeys))
		for t, k := range marshalKeys {
			out[k] = t.Elem() // deref all the pointers so reflect.New gives the correct type
		}
		return out
	}()

	_ Op = &Append{}
	_ Op = &Prepend{}
	_ Op = &Set{}
	_ Op = &Unset{}
	_ Op = &Force{}
	_ Op = &Prefix{}
)

// Append ensures an element exists at the end of a colon separated list.
type Append struct {
	Name  string ` json:"n"`
	Value string ` json:"v"`
}

func (e *Append) sealed() {}
func (e *Append) String() string {
	return fmt.Sprintf(`%s="${%s}:%s"`, e.Name, e.Name, shellquote.Join(e.Value))
}
func (e *Append) Envar() string { return e.Name } // nolint: golint
func (e *Append) Apply(transform *Transform) { // nolint: golint
	value, _ := transform.get(e.Name)
	out := splitAndDrop(value, e.Value)
	out = append(out, e.Value)
	transform.set(e.Name, strings.Join(out, ":"))
}

func (e *Append) Revert(transform *Transform) { // nolint: golint
	value, _ := transform.get(e.Name)
	out := splitAndDrop(value, e.Value)
	transform.set(e.Name, strings.Join(out, ":"))
}

// Prepend ensures an element exists at the beginning of a colon separated list.
type Prepend struct {
	Name  string ` json:"n"`
	Value string ` json:"v"`
}

func (e *Prepend) sealed() {}
func (e *Prepend) String() string {
	return fmt.Sprintf(`%s=%s:${%s}`, e.Name, shellquote.Join(e.Value), e.Name)
}
func (e *Prepend) Envar() string { return e.Name } // nolint: golint
func (e *Prepend) Apply(transform *Transform) { // nolint: golint
	value, _ := transform.get(e.Name)
	prepend := transform.expand(e.Value)
	out := splitAndDrop(value, prepend)
	out = append([]string{prepend}, out...)
	transform.set(e.Name, strings.Join(out, ":"))
}
func (e *Prepend) Revert(transform *Transform) { // nolint: golint
	value, _ := transform.get(e.Name)
	prepend := transform.expand(e.Value)
	out := splitAndDrop(value, prepend)
	transform.set(e.Name, strings.Join(out, ":"))
}

// Prefix ensures the environment variable has the given prefix.
type Prefix struct {
	Name   string ` json:"n"`
	Prefix string ` json:"p"`
}

func (p *Prefix) sealed() {}
func (p *Prefix) String() string {
	return fmt.Sprintf(`%s=%s${%s}`, p.Name, shellquote.Join(p.Prefix), p.Name)
}
func (p *Prefix) Envar() string { return p.Name } // nolint: golint
func (p *Prefix) Apply(transform *Transform) { // nolint: golint
	prefix := transform.expand(p.Prefix)
	if value, ok := transform.get(p.Name); ok && !strings.HasPrefix(value, prefix) {
		transform.set(p.Name, prefix+value)
	}
}
func (p *Prefix) Revert(transform *Transform) { // nolint: golint
	prefix := transform.expand(p.Prefix)
	if value, ok := transform.get(p.Name); ok && strings.HasPrefix(value, prefix) {
		transform.set(p.Name, strings.TrimPrefix(value, prefix))
	}
}

// Set an environment variable.
type Set struct {
	Name  string ` json:"n"`
	Value string ` json:"v"`
}

func (e *Set) sealed()        {}
func (e *Set) String() string { return fmt.Sprintf(`%s="%s"`, e.Name, shellquote.Join(e.Value)) }
func (e *Set) Envar() string  { return e.Name } // nolint: golint
func (e *Set) Apply(transform *Transform) { // nolint: golint
	if value, ok := transform.get(e.Name); ok {
		old := makeRevertKey(transform, e)
		if _, keep := transform.get(old); !keep {
			transform.set(old, value)
		}
	}
	transform.set(e.Name, e.Value)
}
func (e *Set) Revert(transform *Transform) { // nolint: golint
	old := makeRevertKey(transform, e)
	// Check if the user has changed the value and if so, do nothing.
	if currentValue, ok := transform.get(e.Name); ok && currentValue != transform.expand(e.Value) {
		transform.unset(old)
		return
	}
	transform.unset(e.Name)
	if value, ok := transform.get(old); ok {
		transform.set(e.Name, value)
		transform.unset(old)
	}
}

// Unset an environment variable.
type Unset struct {
	Name string ` json:"n"`
}

func (e *Unset) sealed()        {}
func (e *Unset) String() string { return "unset " + e.Name }
func (e *Unset) Envar() string  { return e.Name } // nolint: golint
func (e *Unset) Apply(transform *Transform) { // nolint: golint
	if value, ok := transform.get(e.Name); ok {
		old := makeRevertKey(transform, e)
		transform.set(old, value)
	}
	transform.unset(e.Name)
}
func (e *Unset) Revert(transform *Transform) { // nolint: golint
	old := makeRevertKey(transform, e) // nolint: ifshort
	// If user has subsequently set the environment variable, do nothing.
	if value, ok := transform.get(e.Name); ok && value != "" {
		transform.unset(old)
		return
	}
	transform.unset(e.Name)
	if value, ok := transform.get(old); ok {
		transform.set(e.Name, value)
		transform.unset(old)
	}
}

// Force set/unset an environment variable without preserving or restoring its previous value.
type Force struct {
	Name  string ` json:"n"`
	Value string ` json:"v"`
}

func (f *Force) sealed() {}
func (f *Force) String() string {
	return fmt.Sprintf(`%s="%s"`, f.Name, shellquote.Join(f.Value))
}
func (f *Force) Envar() string { return f.Name } // nolint: golint
func (f *Force) Apply(transform *Transform) { // nolint: golint
	transform.set(f.Name, f.Value)
}

func (f *Force) Revert(transform *Transform) { // nolint: golint
	transform.unset(f.Name)
}

// Split "envar" by ":" and drop "value" from it.
func splitAndDrop(envar string, value string) []string {
	parts := strings.Split(envar, ":")
	values := strings.Split(value, ":")
	out := make([]string, 0, len(parts))
skip:
	for _, elem := range parts {
		for _, valel := range values {
			if elem == valel {
				continue skip
			}
		}
		out = append(out, elem)
	}
	return out
}

var zero = []byte{0}

// Creates a unique and deterministic key for storing a revert, from an Op.
// nolint: errcheck
func makeRevertKey(transform *Transform, op Op) string {
	hash := fnv.New64a()
	hash.Write([]byte(transform.envRoot))
	hash.Write(zero)
	v := reflect.Indirect(reflect.ValueOf(op))
	t := v.Type()
	hash.Write([]byte(t.Name()))
	hash.Write(zero)
	for i := range t.NumField() {
		ft := t.Field(i)
		if ft.Type.Kind() != reflect.String {
			panic("field " + ft.Name + " must be a string")
		}
		value := v.Field(i).String()
		hash.Write([]byte(ft.Name))
		hash.Write(zero)
		hash.Write([]byte(value))
		hash.Write(zero)
	}
	return fmt.Sprintf("_HERMIT_OLD_%s_%X", op.Envar(), hash.Sum(nil))
}

// transform returns a Transform with e as the base.
func transform(envRoot string, e Envars) *Transform {
	return &Transform{
		envRoot: envRoot,
		seed:    e,
		dest:    Envars{},
	}
}
