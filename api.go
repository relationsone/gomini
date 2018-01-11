package gomini

import "github.com/dop251/goja"

type Getter func() (value interface{})
type Setter func(value interface{})

type SecurityInterceptor func(caller Bundle, property string) (accessGranted bool)

type ExtensionBinder func(bundle Bundle, moduleBuilder ModuleBuilder)
type ObjectBinder func(objectBuilder ObjectBuilder)

type KernelModuleDefinition interface {
	ID() string
	Name() string
	ApiDefinitionFile() string
	SecurityInterceptor() SecurityInterceptor
	ExtensionBinder() ExtensionBinder
}

type Origin interface {
	Filename() string
	Path() string
}

type Bundle interface {
	ID() string
	Name() string
	Path() string
	Privileged() bool
	SecurityInterceptor() SecurityInterceptor
	Export(value goja.Value, target interface{}) error

	NewObject() *goja.Object
	ToValue(value interface{}) goja.Value
	Define(property string, value interface{})
	DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter)
	DefineConstant(object *goja.Object, constant string, value interface{})
	PropertyDescriptor(object *goja.Object, property string) (value interface{}, writable bool, getter Getter, setter Setter)
	FreezeObject(object *goja.Object)

	getSandbox() *goja.Runtime
	getBundleExports() *goja.Object
	getAdapter() *adapter
	findModuleById(id string) *module
	findModuleByModuleFile(file string) *module
	addModule(module *module)
	removeModule(module *module)
	peekLoaderStack() string
	popLoaderStack() string
	pushLoaderStack(element string)
}

type Module interface {
	ID() string
	Name() string
	Origin() Origin
	Bundle() Bundle

	export(value goja.Value, target interface{}) error
	getModuleExports() *goja.Object
	setName(name string)
}

type ModuleBuilder interface {
	DefineObject(objectName string, objectBinder ObjectBinder) ModuleBuilder
	DefineFunction(functionName string, function interface{}) ModuleBuilder
	DefineProperty(
		propertyName string,
		value interface{},
		getter func() interface{},
		setter func(value interface{})) ModuleBuilder
	DefineConstant(constantName string, value interface{}) ModuleBuilder
	EndModule()
}

type ObjectBuilder interface {
	DefineObject(objectName string, objectBinder ObjectBinder) ObjectBuilder
	DefineFunction(functionName string, function interface{}) ObjectBuilder
	DefineProperty(
		propertyName string,
		value interface{},
		getter func() interface{},
		setter func(value interface{})) ObjectBuilder
	DefineConstant(constantName string, value interface{}) ObjectBuilder
	EndObject()
}

type ScriptExtension interface {
	ID() string
	Name() string
	ScriptFile() string
	ExtensionBinder() ExtensionBinder
	SecurityInterceptor() SecurityInterceptor
}

type ExportAdapter interface {
	Get(property string) interface{}
}
