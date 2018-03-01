package gomini

import (
	"github.com/go-errors/errors"
	"path/filepath"
	"reflect"
	"github.com/spf13/afero"
	"github.com/apex/log"
	"github.com/efarrer/iothrottler"
)

type bundle struct {
	kernel      *kernel
	id          string
	name        string
	basePath    string
	filesystem  afero.Fs
	status      BundleStatus
	sandbox     Sandbox
	privileges  []string
	privileged  bool
	modules     []*module
	loaderStack []string
	ioPool      *iothrottler.IOThrottlerPool
}

func newBundle(kernel *kernel, basePath string, filesystem afero.Fs, id, name string, privileges []string) (*bundle, error) {
	bundle := &bundle{
		kernel:      kernel,
		id:          id,
		name:        name,
		privileges:  privileges,
		basePath:    basePath,
		filesystem:  filesystem,
		loaderStack: make([]string, 0),
		// TODO Add IO throttling using bundle#ioPool
		// ioPool: iothrottler.NewIOThrottlerPool(iothrottler.BytesPerSecond * 1000),
	}

	bundle.sandbox = kernel.sandboxFactory(bundle)

	bundle.setBundleStatus(BundleStatusInstalled)

	builder := bundle.NewObjectBuilder("")
	builder.DefineGoFunction("<module-init>", "register", bundle.__systemRegister)
	builder.BuildInto("System", bundle.sandbox.Global())

	return bundle, nil
}

func (b *bundle) init(kernel *kernel) error {
	if err := kernel.bundleManager.registerDefaults(b); err != nil {
		return err
	}

	return nil
}

func (b *bundle) Null() Value {
	return b.sandbox.NullValue()
}

func (b *bundle) Undefined() Value {
	return b.sandbox.UndefinedValue()
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

func (b *bundle) Export(value Value, target Any) error {
	return b.sandbox.Export(value, target)
}

func (b *bundle) ToValue(value Any) Value {
	return b.sandbox.ToValue(value)
}

func (b *bundle) FreezeObject(object Object) {
	object.Freeze()
}

func (b *bundle) DeepFreezeObject(object Object) {
	object.DeepFreeze()
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

func (b *bundle) Sandbox() Sandbox {
	return b.sandbox
}

func (b *bundle) NewTypeError(args ...Any) Value {
	return b.sandbox.NewTypeError(args)
}

func (b *bundle) NewObjectBuilder(objectName string) ObjectCreator {
	return b.sandbox.NewObjectCreator(objectName)
}

func (b *bundle) getBasePath() string {
	return b.basePath
}

func (b *bundle) setBundleStatus(status BundleStatus) {
	b.status = status
	log.Infof("Bundle: Status of '%s' changed to %s", b.Name(), status)
}

func (b *bundle) NewObject() Object {
	return b.sandbox.NewObject()
}

func (b *bundle) NewException(err error) Object {
	return b.sandbox.NewError(err)
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

func (b *bundle) __systemRegister(call FunctionCall) Value {
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
	if !argument.IsArray() {
		panic(errors.New("neither string (name) or array (dependencies) was passed as the first parameter"))
	}
	argIndex++

	deps := argument.Export().([]interface{})
	dependencies := make([]string, len(deps))
	for i := 0; i < len(deps); i++ {
		dependencies[i] = deps[i].(string)
	}

	var callback registerCallback
	err := b.sandbox.Export(call.Argument(argIndex), &callback)
	if err != nil {
		panic(err)
	}

	err = b.kernel.registerModule(module, dependencies, callback, b)
	if err != nil {
		panic(err)
	}

	return b.Null()
}
