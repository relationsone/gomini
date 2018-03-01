package gomini

import (
	"github.com/spf13/afero"
)

type KernelModuleBinder func(bundle Bundle, builder ObjectBuilder)
type ApiProviderBinder func(kernel Bundle, bundle Bundle, builder ObjectCreator)

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

	IsAccessible(caller Bundle) error

	export(value Value, target interface{}) error
	getModuleExports() Object
	setName(name string)
}

type ResourceLoader interface {
	LoadResource(kernel *kernel, filesystem afero.Fs, filename string) ([]byte, error)
}
