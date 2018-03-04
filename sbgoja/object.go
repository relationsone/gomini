package sbgoja

import (
	"github.com/dop251/goja"
	"reflect"
	"github.com/relationsone/gomini"
	"github.com/go-errors/errors"
)

func newJsObject(object *goja.Object, sandbox *sandbox) gomini.Object {
	obj := &_object{
		_value: &_value{
			sandbox: sandbox,
			value:   object,
		},
	}
	return obj
}

type _object struct {
	*_value
}

func (o *_object) Get(name string) gomini.Value {
	obj := o.unwrap().(*goja.Object)
	return newJsValue(obj.Get(name), o.sandbox)
}

func (o *_object) PropertyDescriptor(name string) gomini.PropertyDescriptor {
	obj := o.unwrap().(*goja.Object)
	desc := obj.PropertyDescriptor(name)
	return gomini.PropertyDescriptor{
		Original:     desc,
		Enumerable:   gomini.PropertyFlag(desc.Enumerable),
		Configurable: gomini.PropertyFlag(desc.Configurable),
		Writable:     gomini.PropertyFlag(desc.Writable),
		Value:        newJsValue(desc.Value, o.sandbox),
		Setter:       o.toJsSetter(desc.Setter),
		Getter:       o.toJsGetter(desc.Getter),
	}
}

func (o *_object) Freeze() gomini.Object {
	o.sandbox.gojaFreeze(unwrapGojaObject(o))
	return o
}

func (o *_object) DeepFreeze() gomini.Object {
	o.sandbox.gojaDeepFreeze(unwrapGojaObject(o))
	return o
}

func (o *_object) DefineFunction(functionName, propertyName string, function gomini.NativeFunction) gomini.Object {
	return o.defineFunction(functionName, propertyName, adaptJsNativeFunction(function, o.sandbox))
}

func (o *_object) DefineGoFunction(functionName, propertyName string, function gomini.GoFunction) gomini.Object {
	switch t := function.(type) {
	case func(goja.FunctionCall) goja.Value:
		return o.defineFunction(functionName, propertyName, t)
	case func(gomini.FunctionCall) gomini.Value:
		return o.defineFunction(functionName, propertyName, adaptJsNativeFunction(t, o.sandbox))
	}

	value := reflect.ValueOf(function)

	for value.Kind() == reflect.Ptr {
		value = reflect.Indirect(value)
	}

	if value.IsValid() {
		switch value.Kind() {
		case reflect.Func:
			return o.defineFunction(functionName, propertyName, adaptGoFunction(functionName, function, value, o.sandbox))
		}
	}

	panic(errors.New("illegal value passed to DefineGoFunction"))
}

func (o *_object) DefineConstant(constantName string, value interface{}) gomini.Object {
	obj := o.unwrap().(*goja.Object)
	v := unwrapValue(value)
	val := o.sandbox.runtime.ToValue(v)
	if err := obj.DefineDataProperty(constantName, val, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *_object) DefineSimpleProperty(propertyName string, value interface{}) gomini.Object {
	obj := o.unwrap().(*goja.Object)
	v := unwrapValue(value)
	val := o.sandbox.runtime.ToValue(v)
	if err := obj.DefineDataProperty(propertyName, val, goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *_object) DefineObjectProperty(objectName string, objectBinder gomini.ObjectBinder) gomini.Object {
	obj := o.unwrap().(*goja.Object)
	objectCreator := newObjectCreator("", o.sandbox)
	objectBinder(gomini.ObjectBuilder(objectCreator))
	object := objectCreator.Build()
	if err := obj.DefineDataProperty(objectName, unwrapGojaValue(object), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *_object) DefineAccessorProperty(propertyName string, getter gomini.Getter, setter gomini.Setter) gomini.Object {
	obj := o.unwrap().(*goja.Object)
	g := o.sandbox.runtime.ToValue(getter)
	s := o.sandbox.runtime.ToValue(setter)
	if err := obj.DefineAccessorProperty(propertyName, g, s, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *_object) defineFunction(functionName, propertyName string, function interface{}) gomini.Object {
	obj := o.unwrap().(*goja.Object)
	f := o.sandbox.runtime.NewNamedNativeFunction(functionName, function)
	if err := obj.DefineDataProperty(propertyName, f, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *_object) toJsSetter(v goja.Value) gomini.Setter {
	var setter gomini.Setter
	if err := o.sandbox.runtime.ExportTo(v, &setter); err != nil {
		panic(err)
	}
	return setter
}

func (o *_object) toJsGetter(v goja.Value) gomini.Getter {
	var getter gomini.Getter
	if err := o.sandbox.runtime.ExportTo(v, &getter); err != nil {
		panic(err)
	}
	return getter
}
