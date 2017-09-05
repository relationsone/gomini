package gomini

import (
	"path/filepath"
	"github.com/dop251/goja"
	"fmt"
	"reflect"
)

type moduleBuilderImpl struct {
	module Module
	kernel *kernel
	*scriptModuleDefinition
}

type objectBuilderImpl struct {
	module Module
	kernel *kernel
	*scriptObjectDefinition
}

func newModuleBuilder(module Module, kernel *kernel) ModuleBuilder {
	return &moduleBuilderImpl{
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

func (mbi *moduleBuilderImpl) DefineObject(objectName string, objectBinder ObjectBinder) ModuleBuilder {
	definition := &scriptObjectDefinition{
		objectName,
		make([]*scriptObjectDefinition, 0),
		make([]*scriptFunctionDefinition, 0),
		make([]*scriptPropertyDefinition, 0),
		make([]*scriptConstantDefinition, 0),
	}

	objectBuilder := &objectBuilderImpl{
		mbi.module,
		mbi.kernel,
		definition,
	}
	objectBinder(objectBuilder)

	mbi.objects = append(mbi.objects, definition)
	return mbi
}

func (mbi *moduleBuilderImpl) DefineFunction(functionName string, function interface{}) ModuleBuilder {
	mbi.functions = append(mbi.functions, &scriptFunctionDefinition{
		functionName,
		function,
	})
	return mbi
}

func (mbi *moduleBuilderImpl) DefineProperty(
	propertyName string,
	value interface{},
	getter func() interface{},
	setter func(value interface{})) ModuleBuilder {

	mbi.properties = append(mbi.properties, &scriptPropertyDefinition{
		propertyName,
		value,
		getter,
		setter,
	})
	return mbi
}

func (mbi *moduleBuilderImpl) DefineConstant(constantName string, value interface{}) ModuleBuilder {
	mbi.constants = append(mbi.constants, &scriptConstantDefinition{
		constantName,
		value,
	})
	return mbi
}

func (mbi *moduleBuilderImpl) EndModule() {
	mbi.defineModule()
}

func (obi *objectBuilderImpl) DefineObject(objectName string, objectBinder ObjectBinder) ObjectBuilder {
	definition := &scriptObjectDefinition{
		objectName,
		make([]*scriptObjectDefinition, 0),
		make([]*scriptFunctionDefinition, 0),
		make([]*scriptPropertyDefinition, 0),
		make([]*scriptConstantDefinition, 0),
	}

	objectBuilder := &objectBuilderImpl{
		obi.module,
		obi.kernel,
		definition,
	}
	objectBinder(objectBuilder)

	obi.objects = append(obi.objects, definition)
	return obi
}

func (obi *objectBuilderImpl) DefineFunction(functionName string, function interface{}) ObjectBuilder {
	obi.functions = append(obi.functions, &scriptFunctionDefinition{
		functionName,
		function,
	})
	return obi
}

func (obi *objectBuilderImpl) DefineProperty(
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

func (obi *objectBuilderImpl) DefineConstant(constantName string, value interface{}) ObjectBuilder {
	obi.constants = append(obi.constants, &scriptConstantDefinition{
		constantName,
		value,
	})
	return obi
}

func (*objectBuilderImpl) EndObject() {
}

func (mbi *moduleBuilderImpl) defineModule() {
	filename := filepath.Join(mbi.module.Origin().Path(), mbi.module.Origin().Filename())
	moduleName := filepath.Base(filename)

	mbi.kernel.mm.defineKernelModule(mbi.module, moduleName, filename, func(exports *goja.Object) {
		mbi.defineFunctions(exports, mbi.functions)
		mbi.defineProperties(exports, mbi.properties)
		mbi.defineConstants(exports, mbi.constants)
		mbi.defineObjects(exports, mbi.objects)
	})

	if mbi.kernel.kernelDebugging {
		fmt.Println(fmt.Sprintf("Registered builtin module: %s with %s", moduleName, filename))
	}
}

func (mbi *moduleBuilderImpl) defineFunctions(parent *goja.Object, definitions []*scriptFunctionDefinition) {
	for _, function := range definitions {
		parent.Set(function.functionName, function.function)
	}
}

func (mbi *moduleBuilderImpl) defineProperties(parent *goja.Object, definitions []*scriptPropertyDefinition) {
	for _, property := range definitions {
		mbi.module.DefineProperty(parent, property.propertyName, property.value, property.getter, property.setter)
	}
}

func (mbi *moduleBuilderImpl) defineConstants(parent *goja.Object, definitions []*scriptConstantDefinition) {
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
		mbi.module.DefineConstant(parent, constant.constantName, value)
	}
}

func (mbi *moduleBuilderImpl) defineObjects(parent *goja.Object, definitions []*scriptObjectDefinition) {
	for _, object := range definitions {
		mbi.defineObject(parent, object)
	}
}

func (mbi *moduleBuilderImpl) defineObject(parent *goja.Object, definition *scriptObjectDefinition) {
	object := mbi.module.NewObject()

	mbi.defineFunctions(object, definition.functions)
	mbi.defineProperties(object, definition.properties)
	mbi.defineConstants(object, definition.constants)
	mbi.defineObjects(object, definition.objects)

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
