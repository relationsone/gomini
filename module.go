package gomini

import (
	"github.com/dop251/goja"
	"github.com/satori/go.uuid"
	"github.com/go-errors/errors"
	"path/filepath"
	"reflect"
)

func newOrigin(filename string) Origin {
	path := filepath.Dir(filename)
	filename = filepath.Base(filename)
	return &moduleOrigin{
		path:     path,
		filename: filename,
	}
}

type moduleOrigin struct {
	path     string
	filename string
}

func (o *moduleOrigin) Filename() string {
	return o.filename
}

func (o *moduleOrigin) Path() string {
	return o.path
}

func newModule(moduleId, name string, origin Origin, bundle Bundle) (*module, error) {
	if moduleId == "" {
		id, err := uuid.NewV4()
		if err != nil {
			return nil, errors.New(err)
		}
		moduleId = id.String()
	}

	module := &module{
		id:     moduleId,
		name:   name,
		origin: origin,
		bundle: bundle,
		system: bundle.NewObject(),
		exports: &exportAdapter{
			jsExports: bundle.NewObject(),
		},
	}

	register := bundle.ToValue(module.systemRegister)
	err := module.system.DefineDataProperty("register", register, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	if err != nil {
		return nil, err
	}

	return module, nil
}

type module struct {
	id      string
	name    string
	origin  Origin
	bundle  Bundle
	exports *exportAdapter
	system  *goja.Object
}

func (m *module) ID() string {
	return m.id
}

func (m *module) Name() string {
	return m.name
}

func (m *module) Origin() Origin {
	return m.origin
}

func (m *module) Bundle() Bundle {
	return m.bundle
}

func (m *module) ModuleExports() ExportAdapter {
	return m.exports
}

func (m *module) getModuleExports() *goja.Object {
	return m.exports.jsExports
}

func (m *module) Export(value goja.Value, target interface{}) error {
	return m.bundle.getSandbox().ExportTo(value, target)
}

func (m *module) systemRegister(call goja.FunctionCall) goja.Value {
	name := stringModuleOrigin(m)

	argIndex := 0
	argument := call.Argument(argIndex)
	switch argument.ExportType().Kind() {
	case reflect.String:
		name = argument.String()
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
	err := m.Export(call.Argument(argIndex), &callback)
	if err != nil {
		panic(err)
	}

	err = m.bundle.registerModule(name, dependencies, callback, m)
	if err != nil {
		panic(err)
	}

	return goja.Undefined()
}
