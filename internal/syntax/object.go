package syntax

import "strings"

// Object is the internal PDF object model. These types are deliberately not
// exposed outside the module.
type Object interface{ isObject() }

type Null struct{}

type Bool bool

type Int int64

type Real float64

type String []byte

type Name string

type Array []Object

type Dict map[Name]Object

type Stream struct {
	Dict   Dict
	Raw    []byte
	Offset int64
}

type Ref struct {
	Num int
	Gen int
}

func (Null) isObject()    {}
func (Bool) isObject()    {}
func (Int) isObject()     {}
func (Real) isObject()    {}
func (String) isObject()  {}
func (Name) isObject()    {}
func (Array) isObject()   {}
func (Dict) isObject()    {}
func (*Stream) isObject() {}
func (Ref) isObject()     {}

// Get returns the value for name, accepting keys with or without a slash.
func (d Dict) Get(name string) Object {
	if d == nil {
		return Null{}
	}
	name = strings.TrimPrefix(name, "/")
	if v, ok := d[Name(name)]; ok {
		return v
	}
	return Null{}
}

func (d Dict) Has(name string) bool {
	if d == nil {
		return false
	}
	_, ok := d[Name(strings.TrimPrefix(name, "/"))]
	return ok
}

func (d Dict) NameValue(name string) (string, bool) {
	n, ok := d.Get(name).(Name)
	return string(n), ok
}

func (d Dict) IntValue(name string) (int64, bool) {
	switch v := d.Get(name).(type) {
	case Int:
		return int64(v), true
	case Real:
		return int64(v), true
	}
	return 0, false
}

func (d Dict) BoolValue(name string) (bool, bool) {
	v, ok := d.Get(name).(Bool)
	return bool(v), ok
}

func (d Dict) DictValue(name string) (Dict, bool) {
	v, ok := d.Get(name).(Dict)
	return v, ok
}

func (d Dict) ArrayValue(name string) (Array, bool) {
	v, ok := d.Get(name).(Array)
	return v, ok
}

func (d Dict) RefValue(name string) (Ref, bool) {
	v, ok := d.Get(name).(Ref)
	return v, ok
}

func (d Dict) StreamValue(name string) (*Stream, bool) {
	v, ok := d.Get(name).(*Stream)
	return v, ok
}

// AsName, AsArray and AsDict are convenience converters.
func AsName(o Object) (string, bool) {
	n, ok := o.(Name)
	return string(n), ok
}

func AsArray(o Object) (Array, bool) {
	a, ok := o.(Array)
	return a, ok
}

func AsDict(o Object) (Dict, bool) {
	d, ok := o.(Dict)
	return d, ok
}

func AsInt(o Object) (int64, bool) {
	switch v := o.(type) {
	case Int:
		return int64(v), true
	case Real:
		return int64(v), true
	}
	return 0, false
}

func AsNumber(o Object) (float64, bool) {
	switch v := o.(type) {
	case Int:
		return float64(v), true
	case Real:
		return float64(v), true
	}
	return 0, false
}
