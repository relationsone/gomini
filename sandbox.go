package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"github.com/satori/go.uuid"
	"strings"
	"github.com/go-errors/errors"
)

type sandbox struct {
	kernel             *kernel
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

func newSandbox(kernel *kernel, id, name, origin string, privileged bool, parentModule Module) (*sandbox, error) {
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

	sandbox := &sandbox{
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

	sandbox.exports = sandbox.vm.NewObject()
	sandbox.propertyDefiner = prepareDefineProperty(sandbox.vm)
	sandbox.constantDefiner = prepareDefineConstant(sandbox.vm)
	sandbox.propertyDescriptor = preparePropertyDescriptor(sandbox.vm)

	if err := kernel.mm.registerDefaults(sandbox); err != nil {
		return nil, err
	}
	if err := kernel.mm.registerSystemObject(sandbox, parentModule); err != nil {
		return nil, err
	}

	adapter, err := newAdapter(kernel, sandbox)
	if err != nil {
		return nil, errors.New(err)
	}
	sandbox.adapter = adapter

	return sandbox, nil
}

func (s *sandbox) ID() string {
	return s.id
}

func (s *sandbox) Name() string {
	return s.name
}

func (s *sandbox) Origin() Origin {
	return s.origin
}

func (s *sandbox) Exports() map[string]interface{} {
	c := make(map[string]interface{}, len(s.exported))
	for k, v := range s.exported {
		c[k] = v
	}
	return c
}

func (s *sandbox) Privileged() bool {
	return s.privileged
}

func (s *sandbox) SecurityInterceptor() SecurityInterceptor {
	return s.interceptor
}

func (s *sandbox) NewObject() *goja.Object {
	return s.vm.NewObject()
}

func (s *sandbox) Define(property string, value interface{}) {
	s.vm.Set(property, value)
}

func (s *sandbox) DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter) {
	//s.propertyDefiner(object, property, value, getter, setter)
	callPropertyDefiner(s.propertyDefiner, s.vm, object, property, value, getter, setter)
}

func (s *sandbox) DefineConstant(object *goja.Object, constant string, value interface{}) {
	s.constantDefiner(object, constant, value)
}

func (s *sandbox) PropertyDescriptor(object *goja.Object, property string) (interface{}, bool, Getter, Setter) {
	descriptor := s.propertyDescriptor(object, property)
	return propertyDescriptor(s.vm, descriptor.ToObject(s.vm))
}

func (s *sandbox) Export(value goja.Value, target interface{}) error {
	return s.vm.ExportTo(value, target)
}

func (s *sandbox) ToValue(value interface{}) goja.Value {
	return s.vm.ToValue(value)
}

func (s *sandbox) FreezeObject(object *goja.Object) {
	_freezeObject(s.ToValue(object), s.vm)
}

func (s *sandbox) getExports() *goja.Object {
	return s.exports
}

func (s *sandbox) setName(name string) {
	s.name = name
}

func (s *sandbox) getVm() *goja.Runtime {
	return s.vm
}

func (s *sandbox) getAdapter() *adapter {
	return s.adapter
}
