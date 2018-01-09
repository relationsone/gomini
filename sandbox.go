package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"github.com/satori/go.uuid"
	"strings"
	"github.com/go-errors/errors"
)

type moduleImpl struct {
	kernel             *kernel
	bundle             *moduleBundle
	vm                 *goja.Runtime
	privileged         bool
	id                 string
	name               string
	origin             *moduleOrigin
	exports            *goja.Object
	exported           map[string]interface{}
	interceptor        SecurityInterceptor
	propertyDefiner    goja.Callable
	constantDefiner    set_constant
	propertyDescriptor get_property
	adapter            *adapter
}

func newSandbox(kernel *kernel, id, name, origin string, privileged bool, parentModule Module) (*moduleImpl, error) {
	path := filepath.Dir(origin)
	filename := filepath.Base(origin)

	if strings.TrimSpace(id) == "" {
		id = uuid.NewV4().String()
	}

	if strings.TrimSpace(name) == "" {
		name = filename
	}

	if strings.TrimSpace(origin) == "" {
		return nil, errors.New("Given origin location cannot be empty when creating a new script sandbox")
	}

	module := &moduleImpl{
		kernel:     kernel,
		vm:         goja.New(),
		privileged: privileged,
		id:         id,
		name:       name,
		origin: &moduleOrigin{
			filename: filename,
			path:     path,
		},
	}

	module.exports = module.vm.NewObject()
	module.propertyDefiner = prepareDefineProperty(module.vm)
	module.constantDefiner = prepareDefineConstant(module.vm)
	module.propertyDescriptor = preparePropertyDescriptor(module.vm)

	if err := kernel.mm.registerDefaults(module); err != nil {
		return nil, err
	}
	if err := kernel.mm.registerSystemObject(module, parentModule); err != nil {
		return nil, err
	}

	adapter, err := newAdapter(kernel, module)
	if err != nil {
		return nil, errors.New(err)
	}
	module.adapter = adapter

	return module, nil
}

func (s *moduleImpl) ID() string {
	return s.id
}

func (s *moduleImpl) Name() string {
	return s.name
}

func (s *moduleImpl) Origin() Origin {
	return s.origin
}

func (s *moduleImpl) Exports() map[string]interface{} {
	c := make(map[string]interface{}, len(s.exported))
	for k, v := range s.exported {
		c[k] = v
	}
	return c
}

func (s *moduleImpl) Privileged() bool {
	return s.privileged
}

func (s *moduleImpl) SecurityInterceptor() SecurityInterceptor {
	return s.interceptor
}

func (s *moduleImpl) NewObject() *goja.Object {
	return s.vm.NewObject()
}

func (s *moduleImpl) Define(property string, value interface{}) {
	s.vm.Set(property, value)
}

func (s *moduleImpl) DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter) {
	//s.propertyDefiner(object, property, value, getter, setter)
	callPropertyDefiner(s.propertyDefiner, s.vm, object, property, value, getter, setter)
}

func (s *moduleImpl) DefineConstant(object *goja.Object, constant string, value interface{}) {
	s.constantDefiner(object, constant, value)
}

func (s *moduleImpl) PropertyDescriptor(object *goja.Object, property string) (interface{}, bool, Getter, Setter) {
	descriptor := s.propertyDescriptor(object, property)
	return propertyDescriptor(s.vm, descriptor.ToObject(s.vm))
}

func (s *moduleImpl) Export(value goja.Value, target interface{}) error {
	return s.vm.ExportTo(value, target)
}

func (s *moduleImpl) ToValue(value interface{}) goja.Value {
	return s.vm.ToValue(value)
}

func (s *moduleImpl) FreezeObject(object *goja.Object) {
	_freezeObject(s.ToValue(object), s.vm)
}

func (s *moduleImpl) getExports() *goja.Object {
	return s.exports
}

func (s *moduleImpl) setName(name string) {
	s.name = name
}

func (s *moduleImpl) getVm() *goja.Runtime {
	return s.vm
}

func (s *moduleImpl) getAdapter() *adapter {
	return s.adapter
}
