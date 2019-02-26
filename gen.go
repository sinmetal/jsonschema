package jsonschema

import (
	"encoding/json"
	"io"
	"path"
	"reflect"
)

const (
	// RefRoot is root of JSON Schema reference.
	RefRoot = "#/"
)

// Generator generates a JSON Schema.
type Generator interface {
	JSONSchema(w io.Writer, opts ...Option) error
}

// Generate generates JSON Schema from a Go type.
// Channel, complex, and function values cannot be encoded in JSON Schema.
// Attempting to generate such a type causes Generate to return
// an UnsupportedTypeError.
func Generate(w io.Writer, v interface{}, opts ...Option) error {

	if g, ok := v.(Generator); ok {
		return g.JSONSchema(w, opts...)
	}

	var g gen
	o := &obj{
		m:   map[string]interface{}{},
		ref: RefRoot,
	}

	if err := g.do(o, reflect.TypeOf(v), opts...); err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(o.m)
}

type gen struct{}

func (g *gen) do(o Object, t reflect.Type, options ...Option) error {

	switch t.Kind() {
	// unsupported types
	case reflect.Complex64, reflect.Complex128, reflect.Interface,
		reflect.Chan, reflect.Func, reflect.Invalid, reflect.UnsafePointer:
		return &json.UnsupportedTypeError{t}
	case reflect.Ptr:
		return g.do(o, t.Elem(), options...)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64:
		o.Set("type", "number")
	case reflect.Bool:
		o.Set("type", "boolean")
	case reflect.String:
		o.Set("type", "string")
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return &json.UnsupportedTypeError{t}
		}
		o.Set("type", "object")
	case reflect.Array, reflect.Slice:
		if err := g.arrayGen(o, t.Elem(), options...); err != nil {
			return err
		}
	case reflect.Struct:
		if err := g.structGen(o, t, options...); err != nil {
			return err
		}
	}

	for _, opt := range options {
		var err error
		o, err = opt(o)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *gen) arrayGen(parent Object, t reflect.Type, options ...Option) error {
	o := &obj{
		m:   map[string]interface{}{},
		ref: path.Join(parent.Ref(), "items"),
	}

	if err := g.do(o, t, options...); err != nil {
		return err
	}

	parent.Set("type", "array")
	parent.Set("items", o.m)

	return nil
}

func (g *gen) structGen(parent Object, t reflect.Type, options ...Option) error {
	required := make([]string, t.NumField())
	properties := make(map[string]interface{}, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Name

		if f.Anonymous {
			name = f.Type.Name()
		}

		if v, ok := f.Tag.Lookup("json"); ok {
			name = v
		}

		required[i] = name

		o := &obj{
			m:   map[string]interface{}{},
			ref: path.Join(parent.Ref(), "properties", name),
		}

		opts := make([]Option, len(options)+1)
		copy(opts, options)
		opts[len(opts)-1] = ByReference(o.Ref(), PropertyOrder(i))

		if err := g.do(o, f.Type, opts...); err != nil {
			return err
		}

		properties[name] = o.m
	}

	parent.Set("type", "object")
	parent.Set("title", t.Name())
	parent.Set("required", required)
	parent.Set("properties", properties)

	return nil
}
