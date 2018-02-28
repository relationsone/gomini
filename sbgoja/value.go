package sbgoja

import (
	"github.com/dop251/goja"
	"reflect"
	"github.com/relationsone/gomini"
)

func newJsValue(value goja.Value, sandbox *sandbox) gomini.Value {
	if value == nil || value == goja.Null() {
		return sandbox.NullValue()
	}
	if value == goja.Undefined() {
		return sandbox.UndefinedValue()
	}
	if o, ok := value.(*goja.Object); ok {
		return newJsObject(o, sandbox)
	}
	return &_value{
		value:   value,
		sandbox: sandbox,
	}
}

type _value struct {
	value   goja.Value
	sandbox *sandbox
}

func (v *_value) ToInteger() int64 {
	return v.unwrap().ToInteger()
}

func (v *_value) ToFloat() float64 {
	return v.unwrap().ToFloat()
}

func (v *_value) ToBoolean() bool {
	return v.unwrap().ToBoolean()
}

func (v *_value) ToNumber() gomini.Value {
	return newJsValue(v.value.ToNumber(), v.sandbox)
}

func (v *_value) ToString() gomini.Value {
	return newJsValue(v.value.ToString(), v.sandbox)
}

func (v *_value) ToObject() gomini.Object {
	if o, ok := v.value.(*goja.Object); ok {
		return newJsObject(o, v.sandbox)
	}
	o := v.value.ToObject(v.sandbox.runtime)
	return newJsObject(o, v.sandbox)
}

func (v *_value) SameAs(other gomini.Value) bool {
	return v.unwrap().SameAs(unwrapGojaValue(other))
}

func (v *_value) Equals(other gomini.Value) bool {
	return v.unwrap().Equals(unwrapGojaValue(other))
}

func (v *_value) StrictEquals(other gomini.Value) bool {
	return v.unwrap().StrictEquals(unwrapGojaValue(other))
}

func (v *_value) Export() interface{} {
	return v.unwrap().Export()
}

func (v *_value) ExportType() reflect.Type {
	return v.unwrap().ExportType()
}

func (v *_value) String() string {
	return v.unwrap().String()
}

func (v *_value) IsObject() bool {
	_, ok := v.value.(*goja.Object)
	return ok
}

func (v *_value) IsArray() bool {
	return isArray(v)
}

func (v *_value) IsDefined() bool {
	return isDefined(v)
}

func (v *_value) Unwrap() gomini.Any {
	return v.unwrap()
}

func (v *_value) unwrap() goja.Value {
	return v.value
}
