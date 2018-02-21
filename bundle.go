package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"path/filepath"
	"reflect"
	"github.com/spf13/afero"
	"github.com/apex/log"
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
	privileges         []string
	privileged         bool
	modules            []*module
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
		loaderStack: make([]string, 0),
	}

	bundle.setBundleStatus(BundleStatusInstalled)

	system := sandbox.NewObject()
	sandbox.Set("System", system)
	register := sandbox.NewNamedNativeFunction("<module-init>", bundle.__systemRegister)
	err := system.DefineDataProperty("register", register, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	if err != nil {
		return nil, err
	}

	return bundle, nil
}

func (b *bundle) init(kernel *kernel) error {
	adapter, err := newSecurityProxy(b)
	if err != nil {
		return errors.New(err)
	}

	sandbox := b.Sandbox()
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

func (b *bundle) Sandbox() *goja.Runtime {
	return b.sandbox
}

func (b *bundle) getBasePath() string {
	return b.basePath
}

func (b *bundle) getAdapter() *securityProxy {
	return b.adapter
}

func (b *bundle) setBundleStatus(status BundleStatus) {
	b.status = status
	log.Infof("Bundle: Status of '%s' changed to %s", b.Name(), status)
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
	if getter == nil && setter == nil {
		object.DefineDataProperty(property, b.sandbox.ToValue(value), goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	} else {
		object.DefineAccessorProperty(property, b.sandbox.ToValue(getter), b.sandbox.ToValue(setter), goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
}

func (b *bundle) DefineConstant(object *goja.Object, constant string, value interface{}) {
	object.DefineDataProperty(constant, b.sandbox.ToValue(value), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
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
	index := len(b.loaderStack) - 1
	element := b.loaderStack[index]
	b.loaderStack[index] = ""
	b.loaderStack = b.loaderStack[:index]
	return element
}

func (b *bundle) peekLoaderStack() string {
	if len(b.loaderStack) == 0 {
		return ""
	}
	return b.loaderStack[len(b.loaderStack)-1]
}

func (b *bundle) __systemRegister(call goja.FunctionCall) goja.Value {
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
		panic(errors.New("neither string (name) or array (dependencies) was passed as the first parameter"))
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

	err = b.kernel.registerModule(module, dependencies, callback, b)
	if err != nil {
		panic(err)
	}

	return goja.Undefined()
}
