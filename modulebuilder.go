package gomini

import (
	"path/filepath"
	"github.com/dop251/goja"
	"fmt"
	"reflect"
)

type moduleBuilder struct {
	module Module
	kernel *kernel
	*scriptModuleDefinition
}

type objectBuilder struct {
	module Module
	kernel *kernel
	*scriptObjectDefinition
}

func newModuleBuilder(module Module, kernel *kernel) ModuleBuilder {
	return &moduleBuilder{
		module,
		kernel,
		&scriptModuleDefinition{
			module.Name(),
			make([]*scriptObjectDefinition, 0),
			make([]*scriptFunctionDefinition, 0),
			make([]*scriptPropertyDefinition, 0),
			make([]*scriptConstantDefinition, 0),
		},
	}
}

func (mb *moduleBuilder) DefineObject(objectName string, objectBinder ObjectBinder) ModuleBuilder {
	definition := &scriptObjectDefinition{
		objectName,
		make([]*scriptObjectDefinition, 0),
		make([]*scriptFunctionDefinition, 0),
		make([]*scriptPropertyDefinition, 0),
		make([]*scriptConstantDefinition, 0),
	}

	objectBuilder := &objectBuilder{
		mb.module,
		mb.kernel,
		definition,
	}
	objectBinder(objectBuilder)

	mb.objects = append(mb.objects, definition)
	return mb
}

func (mb *moduleBuilder) DefineFunction(functionName string, function interface{}) ModuleBuilder {
	mb.functions = append(mb.functions, &scriptFunctionDefinition{
		functionName,
		function,
	})
	return mb
}

func (mb *moduleBuilder) DefineProperty(
	propertyName string,
	value interface{},
	getter func() interface{},
	setter func(value interface{})) ModuleBuilder {

	mb.properties = append(mb.properties, &scriptPropertyDefinition{
		propertyName,
		value,
		getter,
		setter,
	})
	return mb
}

func (mb *moduleBuilder) DefineConstant(constantName string, value interface{}) ModuleBuilder {
	mb.constants = append(mb.constants, &scriptConstantDefinition{
		constantName,
		value,
	})
	return mb
}

func (mb *moduleBuilder) EndModule() {
	mb.defineModule()
}

func (obi *objectBuilder) DefineObject(objectName string, objectBinder ObjectBinder) ObjectBuilder {
	definition := &scriptObjectDefinition{
		objectName,
		make([]*scriptObjectDefinition, 0),
		make([]*scriptFunctionDefinition, 0),
		make([]*scriptPropertyDefinition, 0),
		make([]*scriptConstantDefinition, 0),
	}

	objectBuilder := &objectBuilder{
		obi.module,
		obi.kernel,
		definition,
	}
	objectBinder(objectBuilder)

	obi.objects = append(obi.objects, definition)
	return obi
}

func (obi *objectBuilder) DefineFunction(functionName string, function interface{}) ObjectBuilder {
	obi.functions = append(obi.functions, &scriptFunctionDefinition{
		functionName,
		function,
	})
	return obi
}

func (obi *objectBuilder) DefineProperty(
	propertyName string,
	value interface{},
	getter func() interface{},
	setter func(value interface{})) ObjectBuilder {

	obi.properties = append(obi.properties, &scriptPropertyDefinition{
		propertyName,
		value,
		getter,
		setter,
	})
	return obi
}

func (obi *objectBuilder) DefineConstant(constantName string, value interface{}) ObjectBuilder {
	obi.constants = append(obi.constants, &scriptConstantDefinition{
		constantName,
		value,
	})
	return obi
}

func (*objectBuilder) EndObject() {
}

func (mb *moduleBuilder) defineModule() {
	filename := filepath.Join(mb.module.Origin().Path(), mb.module.Origin().Filename())

	mb.kernel.defineKernelModule(mb.module, filename, func(exports *goja.Object) {
		mb.defineFunctions(exports, mb.functions)
		mb.defineProperties(exports, mb.properties)
		mb.defineConstants(exports, mb.constants)
		mb.defineObjects(exports, mb.objects)
	})

	if mb.kernel.kernelDebugging {
		fmt.Println(fmt.Sprintf("Registered builtin module: %s with %s", mb.moduleName, filename))
	}
}

func (mb *moduleBuilder) defineFunctions(parent *goja.Object, definitions []*scriptFunctionDefinition) {
	for _, function := range definitions {
		parent.Set(function.functionName, function.function)
	}
}

func (mb *moduleBuilder) defineProperties(parent *goja.Object, definitions []*scriptPropertyDefinition) {
	for _, property := range definitions {
		mb.module.Bundle().DefineProperty(parent, property.propertyName, property.value, property.getter, property.setter)
	}
}

func (mb *moduleBuilder) defineConstants(parent *goja.Object, definitions []*scriptConstantDefinition) {
	for _, constant := range definitions {
		value := constant.value
		x := reflect.ValueOf(value)
		switch x.Kind() {
		case reflect.String:
			value = x.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			value = x.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			value = x.Uint()
		case reflect.Float32, reflect.Float64:
			value = x.Float()
		case reflect.Bool:
			value = x.Bool()
		}
		mb.module.Bundle().DefineConstant(parent, constant.constantName, value)
	}
}

func (mb *moduleBuilder) defineObjects(parent *goja.Object, definitions []*scriptObjectDefinition) {
	for _, object := range definitions {
		mb.defineObject(parent, object)
	}
}

func (mb *moduleBuilder) defineObject(parent *goja.Object, definition *scriptObjectDefinition) {
	object := mb.module.Bundle().NewObject()

	mb.defineFunctions(object, definition.functions)
	mb.defineProperties(object, definition.properties)
	mb.defineConstants(object, definition.constants)
	mb.defineObjects(object, definition.objects)

	parent.Set(definition.objectName, object)
}

type scriptModuleDefinition struct {
	moduleName string
	objects    []*scriptObjectDefinition
	functions  []*scriptFunctionDefinition
	properties []*scriptPropertyDefinition
	constants  []*scriptConstantDefinition
}

type scriptObjectDefinition struct {
	objectName string
	objects    []*scriptObjectDefinition
	functions  []*scriptFunctionDefinition
	properties []*scriptPropertyDefinition
	constants  []*scriptConstantDefinition
}

type scriptFunctionDefinition struct {
	functionName string
	function     interface{}
}

type scriptPropertyDefinition struct {
	propertyName string
	value        interface{}
	getter       func() interface{}
	setter       func(interface{})
}

type scriptConstantDefinition struct {
	constantName string
	value        interface{}
}
