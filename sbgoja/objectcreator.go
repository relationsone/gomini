package sbgoja

import (
	"github.com/relationsone/gomini"
	"github.com/dop251/goja"
	"reflect"
	"github.com/go-errors/errors"
)

type functionDefinition struct {
	propertyName string
	functionName string
	function     gomini.Any
}

type propertyDefinition struct {
	propertyName string
	value        gomini.Any
	getter       gomini.Getter
	setter       gomini.Setter
}

type constantDefinition struct {
	constantName string
	value        gomini.Any
}

type objectDefinition struct {
	objects    []*subObjectDefinition
	functions  []*functionDefinition
	properties []*propertyDefinition
	constants  []*constantDefinition
}

type subObjectDefinition struct {
	objectName   string
	objectBinder gomini.ObjectBinder
}

type objectCreator struct {
	gomini.ObjectBuilder

	sandbox    *sandbox
	objectName string
	*objectDefinition
}

func newObjectCreator(objectName string, sandbox *sandbox) gomini.ObjectCreator {
	return &objectCreator{
		sandbox:    sandbox,
		objectName: objectName,
		objectDefinition: &objectDefinition{
			objects:    make([]*subObjectDefinition, 0),
			functions:  make([]*functionDefinition, 0),
			properties: make([]*propertyDefinition, 0),
			constants:  make([]*constantDefinition, 0),
		},
	}
}

func (oc *objectCreator) DefineFunction(functionName, propertyName string, function gomini.NativeFunction) gomini.ObjectBuilder {
	return oc.defineFunction(functionName, propertyName, adaptJsNativeFunction(function, oc.sandbox))
}

func (oc *objectCreator) DefineGoFunction(functionName, propertyName string, function gomini.GoFunction) gomini.ObjectBuilder {
	switch t := function.(type) {
	case func(goja.FunctionCall) goja.Value:
		return oc.defineFunction(functionName, propertyName, t)
	case func(gomini.FunctionCall) gomini.Value:
		return oc.defineFunction(functionName, propertyName, adaptJsNativeFunction(t, oc.sandbox))
	}

	value := reflect.ValueOf(function)

	for value.Kind() == reflect.Ptr {
		value = reflect.Indirect(value)
	}

	if value.IsValid() {
		switch value.Kind() {
		case reflect.Func:
			return oc.defineFunction(functionName, propertyName, adaptGoFunction(functionName, function, value, oc.sandbox))
		}
	}

	panic(errors.New("illegal _value passed to DefineGoFunction"))
}

func (oc *objectCreator) DefineConstant(constantName string, value gomini.Any) gomini.ObjectBuilder {
	oc.constants = append(oc.constants, &constantDefinition{
		constantName: constantName,
		value:        value,
	})
	return oc
}

func (oc *objectCreator) DefineSimpleProperty(propertyName string, value gomini.Any) gomini.ObjectBuilder {
	oc.properties = append(oc.properties, &propertyDefinition{
		propertyName: propertyName,
		value:        value,
	})
	return oc
}

func (oc *objectCreator) DefineObjectProperty(objectName string, objectBinder gomini.ObjectBinder) gomini.ObjectBuilder {
	oc.objects = append(oc.objects, &subObjectDefinition{
		objectName:   objectName,
		objectBinder: objectBinder,
	})
	return oc
}

func (oc *objectCreator) DefineAccessorProperty(propertyName string, getter gomini.Getter, setter gomini.Setter) gomini.ObjectBuilder {
	oc.properties = append(oc.properties, &propertyDefinition{
		propertyName: propertyName,
		setter:       setter,
		getter:       getter,
	})
	return oc
}

func (oc *objectCreator) Build() gomini.Object {
	object := oc.sandbox.NewObject()
	oc.BuildInto("", object)
	return object
}

func (oc *objectCreator) BuildInto(objectName string, parent gomini.Object) {
	object := parent
	if objectName != "" {
		object = oc.sandbox.NewObject()
	}

	oc.buildProperties(object)
	oc.buildConstants(object)
	oc.buildFunctions(object)
	oc.buildObjects(object)

	if objectName != "" {
		parent.DefineConstant(objectName, object)
	}
}

func (oc *objectCreator) defineFunction(functionName, propertyName string, function interface{}) gomini.ObjectBuilder {
	oc.functions = append(oc.functions, &functionDefinition{
		propertyName,
		functionName,
		function,
	})
	return oc
}

func (oc *objectCreator) buildObjects(parent gomini.Object) {
	for _, subobject := range oc.objects {
		objectCreator := newObjectCreator(subobject.objectName, oc.sandbox)
		subobject.objectBinder(gomini.ObjectBuilder(objectCreator))
		objectCreator.BuildInto(subobject.objectName, parent)
	}
}

func (oc *objectCreator) buildFunctions(parent gomini.Object) {
	for _, function := range oc.functions {
		name := function.functionName
		if oc.objectName != "" {
			name = oc.objectName + "." + name
		}
		value := oc.sandbox.NewNamedNativeFunction(name, function.function)
		parent.DefineConstant(function.propertyName, value)
	}
}

func (oc *objectCreator) buildProperties(parent gomini.Object) {
	for _, property := range oc.properties {
		name := property.propertyName
		if property.getter == nil && property.setter == nil {
			v := unwrapValue(property.value)
			value := oc.sandbox.ToValue(v)
			if err := parent.DefineSimpleProperty(name, value); err != nil {
				panic(err)
			}
		} else {
			parent.DefineAccessorProperty(name, property.getter, property.setter)
		}
	}
}

func (oc *objectCreator) buildConstants(parent gomini.Object) {
	for _, constant := range oc.constants {
		v := unwrapValue(constant.value)
		name := constant.constantName
		value := oc.sandbox.ToValue(v)
		parent.DefineConstant(name, value)
	}
}
