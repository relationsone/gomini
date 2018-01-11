package gomini

import (
	"github.com/dop251/goja"
	"github.com/satori/go.uuid"
	"github.com/go-errors/errors"
	"path/filepath"
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

func (o *moduleOrigin) FullPath() string {
	return filepath.Clean(filepath.Join(o.path, o.filename))
}

type module struct {
	id      string
	name    string
	origin  Origin
	bundle  Bundle
	exports *goja.Object
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
		id:      moduleId,
		name:    name,
		origin:  origin,
		bundle:  bundle,
		exports: bundle.NewObject(),
	}

	return module, nil
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

func (m *module) getModuleExports() *goja.Object {
	return m.exports
}

func (m *module) export(value goja.Value, target interface{}) error {
	return m.bundle.getSandbox().ExportTo(value, target)
}

func (m *module) setName(name string) {
	m.name = name
}
