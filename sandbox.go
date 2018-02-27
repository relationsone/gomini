package gomini

import (
	"github.com/dop251/goja"
	"reflect"
	"github.com/go-errors/errors"
)

var (
	typeJsCallable = reflect.TypeOf((*JsCallable)(nil)).Elem()
	typeJsObject   = reflect.TypeOf((*JsObject)(nil)).Elem()
	typeJsValue    = reflect.TypeOf((*JsValue)(nil)).Elem()
	typeGojaObject = reflect.TypeOf(&goja.Object{})
	typeGojaValue  = reflect.TypeOf((*goja.Value)(nil)).Elem()
)

func newObjectCreator(objectName string, assignee *goja.Object, bundle Bundle) *JsObjectCreator {
	return &JsObjectCreator{
		bundle:     bundle,
		objectName: objectName,
		assignee:   assignee,
		jsObjectDefinition: &jsObjectDefinition{
			objects:    make([]*jsSubObjectDefinition, 0),
			functions:  make([]*jsFunctionDefinition, 0),
			properties: make([]*jsPropertyDefinition, 0),
			constants:  make([]*jsConstantDefinition, 0),
		},
	}
}

func (oc *JsObjectCreator) DefineFunction(functionName string, function JsNativeFunction) JsObjectBuilder {
	return oc.defineFunction(functionName, adaptJsNativeFunction(function, oc.bundle))
}

func (oc *JsObjectCreator) DefineGoFunction(functionName string, function JsGoFunction) JsObjectBuilder {
	switch t := function.(type) {
	case func(call goja.FunctionCall) goja.Value:
		return oc.defineFunction(functionName, t)
	case JsNativeFunction:
		return oc.defineFunction(functionName, adaptJsNativeFunction(t, oc.bundle))
	}

	value := reflect.ValueOf(function)

	for value.Kind() == reflect.Ptr {
		value = reflect.Indirect(value)
	}

	if value.IsValid() {
		switch value.Kind() {
		case reflect.Func:
			return oc.defineFunction(functionName, adaptGoFunction(functionName, function, value, oc.bundle))
		}
	}

	panic(errors.New("illegal value passed to DefineGoFunction"))
}

func (oc *JsObjectCreator) DefineConstant(constantName string, value Any) JsObjectBuilder {
	oc.constants = append(oc.constants, &jsConstantDefinition{
		constantName: constantName,
		value:        value,
	})
	return oc
}

func (oc *JsObjectCreator) DefineSimpleProperty(propertyName string, value Any) JsObjectBuilder {
	oc.properties = append(oc.properties, &jsPropertyDefinition{
		propertyName: propertyName,
		value:        value,
	})
	return oc
}

func (oc *JsObjectCreator) DefineObjectProperty(objectName string, objectBinder JsObjectBinder) JsObjectBuilder {
	oc.objects = append(oc.objects, &jsSubObjectDefinition{
		objectName:   objectName,
		objectBinder: objectBinder,
	})
	return oc
}

func (oc *JsObjectCreator) DefineAccessorProperty(propertyName string, getter JsGetter, setter JsSetter) JsObjectBuilder {
	oc.properties = append(oc.properties, &jsPropertyDefinition{
		propertyName: propertyName,
		setter:       setter,
		getter:       getter,
	})
	return oc
}

func (oc *JsObjectCreator) Build() JsObject {
	object := oc.assignee
	if object == nil {
		object = oc.bundle.NewObject()
	}
	oc.buildProperties(object)
	oc.buildConstants(object)
	oc.buildFunctions(object)
	oc.buildObjects(object)
	return newJsObject(object, oc.bundle)
}

func (oc *JsObjectCreator) defineFunction(functionName string, function interface{}) JsObjectBuilder {
	oc.functions = append(oc.functions, &jsFunctionDefinition{
		functionName,
		function,
	})
	return oc
}

func (oc *JsObjectCreator) buildObjects(parent *goja.Object) {
	for _, subobject := range oc.objects {
		objectCreator := newObjectCreator(subobject.objectName, nil, oc.bundle)
		subobject.objectBinder(JsObjectBuilder(objectCreator))
		object := objectCreator.Build()
		if err := parent.DefineDataProperty(subobject.objectName, object.unwrap(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			panic(err)
		}
	}
}

func (oc *JsObjectCreator) buildFunctions(parent *goja.Object) {
	for _, function := range oc.functions {
		name := function.functionName
		if oc.objectName != "" {
			name = oc.objectName + "." + name
		}
		value := oc.bundle.Sandbox().NewNamedNativeFunction(name, function.function)
		if err := parent.DefineDataProperty(function.functionName, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			panic(err)
		}
	}
}

func (oc *JsObjectCreator) buildProperties(parent *goja.Object) {
	sandbox := oc.bundle.Sandbox()
	for _, property := range oc.properties {
		name := property.propertyName
		if property.getter == nil && property.setter == nil {
			v := unwrapValue(property.value)
			value := sandbox.ToValue(v)
			if err := parent.DefineDataProperty(name, value, goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
				panic(err)
			}
		} else {
			getter := sandbox.ToValue(property.getter)
			setter := sandbox.ToValue(property.setter)
			if err := parent.DefineAccessorProperty(name, getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
				panic(err)
			}
		}
	}
}

func (oc *JsObjectCreator) buildConstants(parent *goja.Object) {
	for _, constant := range oc.constants {
		v := unwrapValue(constant.value)
		name := constant.constantName
		value := oc.bundle.Sandbox().ToValue(v)
		if err := parent.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			panic(err)
		}
	}
}

func newJsObject(object *goja.Object, bundle Bundle) JsObject {
	obj := &__jsObject{
		__jsValue: &__jsValue{
			bundle: bundle,
			value:  object,
		},
	}
	return obj
}

type __jsObject struct {
	*__jsValue
}

func (o *__jsObject) PropertyDescriptor(name string) JsPropertyDescriptor {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	desc := obj.PropertyDescriptor(name)
	return JsPropertyDescriptor{
		original:     desc,
		Enumerable:   JsPropertyFlag(desc.Enumerable),
		Configurable: JsPropertyFlag(desc.Configurable),
		Writable:     JsPropertyFlag(desc.Writable),
		Value:        newJsValue(desc.Value, o.bundle),
		Setter:       o.toJsSetter(desc.Setter),
		Getter:       o.toJsGetter(desc.Getter),
	}
}

func (o *__jsObject) toJsSetter(v goja.Value) JsSetter {
	var setter JsSetter
	if err := o.bundle.Export(v, &setter); err != nil {
		panic(err)
	}
	return setter
}

func (o *__jsObject) toJsGetter(v goja.Value) JsGetter {
	var getter JsGetter
	if err := o.bundle.Export(v, &getter); err != nil {
		panic(err)
	}
	return getter
}

func (o *__jsObject) Freeze() JsObject {
	o.bundle.DeepFreezeObject(o.unwrap().ToObject(o.bundle.Sandbox()))
	return o
}

func (o *__jsObject) DefineFunction(functionName string, function JsNativeFunction) JsObject {
	return o.defineFunction(functionName, adaptJsNativeFunction(function, o.bundle))
}

func (o *__jsObject) DefineGoFunction(functionName string, function JsGoFunction) JsObject {
	switch t := function.(type) {
	case func(call goja.FunctionCall) goja.Value:
		return o.defineFunction(functionName, t)
	case JsNativeFunction:
		return o.defineFunction(functionName, adaptJsNativeFunction(t, o.bundle))
	}

	value := reflect.ValueOf(function)

	for value.Kind() == reflect.Ptr {
		value = reflect.Indirect(value)
	}

	if value.IsValid() {
		switch value.Kind() {
		case reflect.Func:
			return o.defineFunction(functionName, adaptGoFunction(functionName, function, value, o.bundle))
		}
	}

	panic(errors.New("illegal value passed to DefineGoFunction"))
}

func (o *__jsObject) defineFunction(functionName string, function Any) JsObject {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	f := o.bundle.Sandbox().NewNamedNativeFunction(functionName, function)
	if err := obj.DefineDataProperty(functionName, f, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *__jsObject) DefineConstant(constantName string, value Any) JsObject {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	v := unwrapValue(value)
	val := o.bundle.Sandbox().ToValue(v)
	if err := obj.DefineDataProperty(constantName, val, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *__jsObject) DefineSimpleProperty(propertyName string, value Any) JsObject {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	v := unwrapValue(value)
	val := o.bundle.Sandbox().ToValue(v)
	if err := obj.DefineDataProperty(propertyName, val, goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *__jsObject) DefineObjectProperty(objectName string, objectBinder JsObjectBinder) JsObject {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	objectCreator := newObjectCreator(objectName, nil, o.bundle)
	objectBinder(JsObjectBuilder(objectCreator))
	object := objectCreator.Build()
	if err := obj.DefineDataProperty(objectName, object.unwrap(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func (o *__jsObject) DefineAccessorProperty(propertyName string, getter JsGetter, setter JsSetter) JsObject {
	obj, ok := o.unwrap().(*goja.Object)
	if !ok {
		panic(errors.New("not an object"))
	}
	g := o.bundle.Sandbox().ToValue(getter)
	s := o.bundle.Sandbox().ToValue(setter)
	if err := obj.DefineAccessorProperty(propertyName, g, s, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		panic(err)
	}
	return o
}

func newJsValue(value goja.Value, bundle Bundle) JsValue {
	if value == nil || value == goja.Null() {
		return bundle.Null()
	}
	if value == goja.Undefined() {
		return bundle.Undefined()
	}
	if o, ok := value.(*goja.Object); ok {
		return newJsObject(o, bundle)
	}
	return &__jsValue{
		value:  value,
		bundle: bundle,
	}
}

type __jsValue struct {
	value  goja.Value
	bundle Bundle
}

func (b *__jsValue) ToInteger() int64 {
	return b.unwrap().ToInteger()
}

func (b *__jsValue) ToFloat() float64 {
	return b.unwrap().ToFloat()
}

func (b *__jsValue) ToBoolean() bool {
	return b.unwrap().ToBoolean()
}

func (b *__jsValue) ToNumber() JsValue {
	v := b.unwrap().ToNumber()
	return newJsValue(v, b.bundle)
}

func (b *__jsValue) ToString() JsValue {
	v := b.unwrap().ToString()
	return newJsValue(v, b.bundle)
}

func (b *__jsValue) ToObject() JsObject {
	if o, ok := b.value.(*goja.Object); ok {
		return newJsObject(o, b.bundle)
	}
	o := b.value.ToObject(b.bundle.Sandbox())
	return newJsObject(o, b.bundle)
}

func (b *__jsValue) SameAs(other JsValue) bool {
	return b.unwrap().SameAs(other.unwrap())
}

func (b *__jsValue) Equals(other JsValue) bool {
	return b.unwrap().Equals(other.unwrap())
}

func (b *__jsValue) StrictEquals(other JsValue) bool {
	return b.unwrap().StrictEquals(other.unwrap())
}

func (b *__jsValue) Export() interface{} {
	return b.unwrap().Export()
}

func (b *__jsValue) ExportType() reflect.Type {
	return b.unwrap().ExportType()
}

func (b *__jsValue) String() string {
	return b.unwrap().String()
}

func (b *__jsValue) unwrap() goja.Value {
	return b.value
}
