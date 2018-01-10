package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"path/filepath"
)

func newBundle(kernel *kernel, id, name, basePath string) (*bundle, error) {
	sandbox := goja.New()

	bundle := &bundle{
		kernel:   kernel,
		id:       id,
		name:     name,
		basePath: basePath,
		sandbox:  sandbox,
		exports: &exportAdapter{
			goExports: make(map[string]interface{}),
			jsExports: sandbox.NewObject(),
		},
	}

	adapter, err := newAdapter(kernel, bundle)
	if err != nil {
		return nil, errors.New(err)
	}

	bundle.propertyDefiner = prepareDefineProperty(sandbox)
	bundle.constantDefiner = prepareDefineConstant(sandbox)
	bundle.propertyDescriptor = preparePropertyDescriptor(sandbox)

	if err := kernel.bundleManager.registerDefaults(bundle); err != nil {
		return nil, err
	}

	bundle.adapter = adapter

	return bundle, nil
}

type bundle struct {
	kernel             *kernel
	id                 string
	name               string
	basePath           string
	sandbox            *goja.Runtime
	adapter            *adapter
	exports            *exportAdapter
	privileged         bool
	modules            []*module
	propertyDefiner    goja.Callable
	constantDefiner    set_constant
	propertyDescriptor get_property
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

func (b *bundle) Path() string {
	return b.basePath
}

func (b *bundle) BundleExports() ExportAdapter {
	return b.exports
}

func (b *bundle) getBundleExports() *goja.Object {
	return b.exports.jsExports
}

func (b *bundle) Privileged() bool {
	return b.privileged
}

func (b *bundle) SecurityInterceptor() SecurityInterceptor {
	return func(caller Bundle, property string) (accessGranted bool) {
		// TODO: Implement a real security check here! For now make it easy and get it running again
		return true
	}
}

func (b *bundle) getSandbox() *goja.Runtime {
	return b.sandbox
}

func (b *bundle) getAdapter() *adapter {
	return b.adapter
}

func (b *bundle) NewObject() *goja.Object {
	return b.sandbox.NewObject()
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

func (b *bundle) registerModule(name string, dependencies []string, callback registerCallback, module Module) error {
	return b.kernel.kernelRegisterModule(name, dependencies, callback, module, b)
}
