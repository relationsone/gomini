package gomini

import "github.com/spf13/afero"

type ResourceLoader interface {
	LoadResource(kernel *kernel, filesystem afero.Fs, filename string) ([]byte, error)
}

type SecurityInterceptor func(caller Bundle, property string) (accessGranted bool)

type ApiProviderBinder func(kernel Bundle, bundle Bundle, builder ObjectCreator)

type KernelModuleBinder func(bundle Bundle, builder ObjectBuilder)
