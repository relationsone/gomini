package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"path/filepath"
	"reflect"
	"github.com/spf13/afero"
)

type bundle struct {
	kernel             *kernel
	id                 string
	name               string
	basePath           string
	filesystem         afero.Fs
	status             BundleStatus
	sandbox            *goja.Runtime
	adapter            *securityProxy
	exports            *goja.Object
	privileges         []string
	privileged         bool
	modules            []*module
	propertyDefiner    goja.Callable
	constantDefiner    set_constant
	propertyDescriptor get_property
	loaderStack        []string
}

func newBundle(kernel *kernel, basePath string, filesystem afero.Fs, id, name string, privileges []string) (*bundle, error) {
	sandbox := goja.New()

	bundle := &bundle{
		kernel:      kernel,
		id:          id,
		name:        name,
		privileges:  privileges,
		basePath:    basePath,
		filesystem:  filesystem,
		sandbox:     sandbox,
		exports:     sandbox.NewObject(),
		loaderStack: make([]string, 0),
	}

	system := sandbox.NewObject()
	sandbox.Set("System", system)
	register := sandbox.ToValue(bundle.systemRegister)
	err := system.DefineDataProperty("register", register, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (b *bundle) init(kernel *kernel) error {
	adapter, err := newSecurityProxy(kernel, b)
	if err != nil {
		return errors.New(err)
	}

	sandbox := b.getSandbox()
	b.propertyDefiner = prepareDefineProperty(sandbox)
	b.constantDefiner = prepareDefineConstant(sandbox)
	b.propertyDescriptor = preparePropertyDescriptor(sandbox)

	if err := kernel.bundleManager.registerDefaults(b); err != nil {
		return err
	}

	b.adapter = adapter

	return nil
}

func (b *bundle) Filesystem() afero.Fs {
	return b.filesystem
}

func (b *bundle) Status() BundleStatus {
	return b.status
}

func (b *bundle) findModuleByModuleFile(file string) *module {
	filename := filepath.Base(file)
	path := filepath.Dir(file)
	for _, module := range b.modules {
		if module.Origin().Filename() == filename && module.Origin().Path() == path {
			return module
		}
	}
	return nil
}

func (b *bundle) findModuleByName(name string) *module {
	for _, module := range b.modules {
		if module.Name() == name {
			return module
		}
	}
	return nil
}

func (b *bundle) findModuleById(id string) *module {
	for _, module := range b.modules {
		if module.ID() == id {
			return module
		}
	}
	return nil
}

func (b *bundle) Export(value goja.Value, target interface{}) error {
	return b.sandbox.ExportTo(value, target)
}

func (b *bundle) ToValue(value interface{}) goja.Value {
	return b.sandbox.ToValue(value)
}

func (b *bundle) FreezeObject(object *goja.Object) {
	_freezeObject(b.ToValue(object), b.sandbox)
}

func (b *bundle) ID() string {
	return b.id
}

func (b *bundle) Name() string {
	return b.name
}

func (b *bundle) getBundleExports() *goja.Object {
	return b.exports
}

func (b *bundle) Privileged() bool {
	return b.privileged
}

func (b *bundle) Privileges() []string {
	return b.privileges
}

func (b *bundle) SecurityInterceptor() SecurityInterceptor {
	return func(caller Bundle, property string) (accessGranted bool) {
		// TODO: Implement a real security check here! For now make it easy and get it running again
		return true
	}
}

func (b *bundle) getBasePath() string {
	return b.basePath
}

func (b *bundle) getSandbox() *goja.Runtime {
	return b.sandbox
}

func (b *bundle) getAdapter() *securityProxy {
	return b.adapter
}

func (b *bundle) NewObject() *goja.Object {
	return b.sandbox.NewObject()
}

func (b *bundle) NewException(err error) *goja.Object {
	return b.sandbox.NewGoError(err)
}

func (b *bundle) Define(property string, value interface{}) {
	b.sandbox.Set(property, value)
}

func (b *bundle) DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter) {
	callPropertyDefiner(b.propertyDefiner, b.sandbox, object, property, value, getter, setter)
}

func (b *bundle) DefineConstant(object *goja.Object, constant string, value interface{}) {
	b.constantDefiner(object, constant, value)
}

func (b *bundle) PropertyDescriptor(object *goja.Object, property string) (value interface{}, writable bool, getter Getter, setter Setter) {
	descriptor := b.propertyDescriptor(object, property)
	return propertyDescriptor(b.sandbox, descriptor.ToObject(b.sandbox))
}

func (b *bundle) addModule(module *module) {
	b.modules = append(b.modules, module)
}

func (b *bundle) removeModule(module *module) {
	for i, el := range b.modules {
		if el == module {
			b.modules = append(b.modules[:i], b.modules[i+1:]...)
			break
		}
	}
}

func (b *bundle) pushLoaderStack(element string) {
	b.loaderStack = append(b.loaderStack, element)
}

func (b *bundle) popLoaderStack() string {
	if len(b.loaderStack) == 0 {
		return ""
	}
	element := b.loaderStack[len(b.loaderStack)-1]
	b.loaderStack = b.loaderStack[:len(b.loaderStack)-1]
	return element
}

func (b *bundle) peekLoaderStack() string {
	if len(b.loaderStack) == 0 {
		return ""
	}
	return b.loaderStack[len(b.loaderStack)-1]
}

func (b *bundle) systemRegister(call goja.FunctionCall) goja.Value {
	var module *module = nil
	if len(b.loaderStack) > 0 {
		moduleId := b.peekLoaderStack()
		module = b.findModuleById(moduleId)
	}

	if module == nil {
		panic(errors.New("failed to load module: internal error"))
	}

	argIndex := 0
	argument := call.Argument(argIndex)
	switch argument.ExportType().Kind() {
	case reflect.String:
		moduleName := argument.String()
		module.setName(moduleName)
		argIndex++
	}

	argument = call.Argument(argIndex)
	if !isArray(argument) {
		panic("Neither string (name) or array (dependencies) was passed as the first parameter")
	}
	argIndex++

	deps := argument.Export().([]interface{})
	dependencies := make([]string, len(deps))
	for i := 0; i < len(deps); i++ {
		dependencies[i] = deps[i].(string)
	}

	var callback registerCallback
	err := b.sandbox.ExportTo(call.Argument(argIndex), &callback)
	if err != nil {
		panic(err)
	}

	err = b.kernel.kernelRegisterModule(module, dependencies, callback, b)
	if err != nil {
		panic(err)
	}

	return goja.Undefined()
}
