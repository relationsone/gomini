package gomini

import (
	"path/filepath"
	"github.com/dop251/goja"
	"reflect"
	"github.com/apex/log"
)

type apiBuilder struct {
	module Module
	bundle Bundle
	kernel *kernel
	*scriptModuleDefinition
}

type objectBuilder struct {
	module Module
	bundle Bundle
	kernel *kernel
	*scriptObjectDefinition
}

func newApiBuilder(module Module, bundle Bundle, kernel *kernel) ApiBuilder {
	name := ""
	if module != nil {
		name = module.Name()
	}

	return &apiBuilder{
		module,
		bundle,
		kernel,
		&scriptModuleDefinition{
			name,
			make([]*scriptObjectDefinition, 0),
			make([]*scriptFunctionDefinition, 0),
			make([]*scriptPropertyDefinition, 0),
			make([]*scriptConstantDefinition, 0),
		},
	}
}

func (ab *apiBuilder) DefineObject(objectName string, objectBinder ObjectBinder) ApiBuilder {
	definition := &scriptObjectDefinition{
		objectName,
		make([]*scriptObjectDefinition, 0),
		make([]*scriptFunctionDefinition, 0),
		make([]*scriptPropertyDefinition, 0),
		make([]*scriptConstantDefinition, 0),
	}

	objectBuilder := &objectBuilder{
		ab.module,
		ab.bundle,
		ab.kernel,
		definition,
	}
	objectBinder(objectBuilder)

	ab.objects = append(ab.objects, definition)
	return ab
}

func (ab *apiBuilder) DefineFunction(functionName string, function interface{}) ApiBuilder {
	ab.functions = append(ab.functions, &scriptFunctionDefinition{
		functionName,
		function,
	})
	return ab
}

func (ab *apiBuilder) DefineProperty(
	propertyName string,
	value interface{},
	getter func() interface{},
	setter func(value interface{})) ApiBuilder {

	ab.properties = append(ab.properties, &scriptPropertyDefinition{
		propertyName,
		value,
		getter,
		setter,
	})
	return ab
}

func (ab *apiBuilder) DefineConstant(constantName string, value interface{}) ApiBuilder {
	ab.constants = append(ab.constants, &scriptConstantDefinition{
		constantName,
		value,
	})
	return ab
}

func (ab *apiBuilder) EndApi() {
	ab.defineModule()
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
		obi.bundle,
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

func (ab *apiBuilder) defineModule() {
	if ab.module != nil {
		filename := filepath.Join(ab.module.Origin().Path(), ab.module.Origin().Filename())

		ab.kernel.defineKernelModule(ab.module, filename, func(exports *goja.Object) {
			ab.defineFunctions(exports, ab.functions)
			ab.defineProperties(exports, ab.properties)
			ab.defineConstants(exports, ab.constants)
			ab.defineObjects(exports, ab.objects)
		})

		log.Infof("ApiBuilder: Registered builtin module: %s (%s) with %s", ab.moduleName, ab.module.ID(), filename)
	} else {
		global := ab.bundle.Sandbox().GlobalObject()
		ab.defineFunctions(global, ab.functions)
		ab.defineProperties(global, ab.properties)
		ab.defineConstants(global, ab.constants)
		ab.defineObjects(global, ab.objects)
	}
}

func (ab *apiBuilder) defineFunctions(parent *goja.Object, definitions []*scriptFunctionDefinition) {
	for _, function := range definitions {
		value := ab.bundle.ToValue(function.function)
		parent.DefineDataProperty(function.functionName, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
}

func (ab *apiBuilder) defineProperties(parent *goja.Object, definitions []*scriptPropertyDefinition) {
	sandbox := ab.bundle.Sandbox()
	for _, property := range definitions {
		name := property.propertyName
		if property.getter == nil && property.setter == nil {
			v := property.value
			x := reflect.ValueOf(v)
			switch x.Kind() {
			case reflect.String:
				v = x.String()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				v = x.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				v = x.Uint()
			case reflect.Float32, reflect.Float64:
				v = x.Float()
			case reflect.Bool:
				v = x.Bool()
			}

			value := sandbox.ToValue(v)
			parent.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		} else {
			getter := sandbox.ToValue(property.getter)
			setter := sandbox.ToValue(property.setter)
			parent.DefineAccessorProperty(name, getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
		}
	}
}

func (ab *apiBuilder) defineConstants(parent *goja.Object, definitions []*scriptConstantDefinition) {
	for _, constant := range definitions {
		v := constant.value
		x := reflect.ValueOf(v)
		switch x.Kind() {
		case reflect.String:
			v = x.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v = x.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v = x.Uint()
		case reflect.Float32, reflect.Float64:
			v = x.Float()
		case reflect.Bool:
			v = x.Bool()
		}

		name := constant.constantName
		value := ab.bundle.ToValue(v)
		parent.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
}

func (ab *apiBuilder) defineObjects(parent *goja.Object, definitions []*scriptObjectDefinition) {
	for _, object := range definitions {
		ab.defineObject(parent, object)
	}
}

func (ab *apiBuilder) defineObject(parent *goja.Object, definition *scriptObjectDefinition) {
	object := ab.bundle.NewObject()

	ab.defineFunctions(object, definition.functions)
	ab.defineProperties(object, definition.properties)
	ab.defineConstants(object, definition.constants)
	ab.defineObjects(object, definition.objects)

	parent.DefineDataProperty(definition.objectName, object, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
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
