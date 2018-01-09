package gomini

import "github.com/dop251/goja"

type moduleOrigin struct {
	filename string
	path     string
}

func (o *moduleOrigin) Filename() string {
	return o.filename
}

func (o *moduleOrigin) Path() string {
	return o.path
}

type moduleBundle struct {
	baseDir       string
	vm            *goja.Runtime
	exports       *goja.Object
	publicExports *goja.Object
}
