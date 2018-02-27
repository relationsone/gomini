package gomini

import (
	"github.com/dop251/goja"
	"github.com/spf13/afero"
)

type KernelModuleBinder func(bundle Bundle, builder JsObjectBuilder)
type ApiProviderBinder func(kernel Bundle, bundle Bundle, builder BundleObjectBuilder)

type SecurityInterceptor func(caller Bundle, property string) (accessGranted bool)

type KernelModule interface {
	ID() string
	Name() string
	ApiDefinitionFile() string
	SecurityInterceptor() SecurityInterceptor
	KernelModuleBinder() KernelModuleBinder
}

type Origin interface {
	Filename() string
	Path() string
	FullPath() string
}

type Module interface {
	ID() string
	Name() string
	Origin() Origin
	Bundle() Bundle

	export(value goja.Value, target Any) error
	getModuleExports() *goja.Object
	setName(name string)
}

type ResourceLoader interface {
	LoadResource(kernel *kernel, filesystem afero.Fs, filename string) ([]byte, error)
}
