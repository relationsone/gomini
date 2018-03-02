package gomini

import (
	"github.com/spf13/afero"
)

const (
	KernelVfsAppsPath     = "/kernel/apps"
	KernelVfsCachePath    = "/kernel/cache"
	KernelVfsTypesPath    = "/kernel/@types"
	KernelVfsWritablePath = "/kernel/data"
)

type KeyManager interface {
	GetKey(fingerprint string) ([]byte, error)
}

type KernelConfig struct {
	NewKernelFilesystem func(baseFilesystem afero.Fs) (afero.Fs, error)
	NewBundleFilesystem func(bundleFilesystemConfig BundleFilesystemConfig) (afero.Fs, error)
	NewSandbox          func(bundle Bundle) Sandbox
	KernelModules       []KernelModule
	BundleApiProviders  []ApiProviderBinder
}

type KernelModule interface {
	ID() string
	Name() string
	SecurityInterceptor() SecurityInterceptor
	KernelModuleBinder() KernelModuleBinder
}

// Kernel is the root Bundle implementation and has special
// privileges. Also the kernel provides access to all native APIs
// provided by either builtin modules or loaded external kernel
// module plugins.
type Kernel interface {
	Bundle

	// Start starts the kernel after all KernelModules by executing
	// the given JavaScript (*.js) or TypeScript (*.ts) file path,
	// which is relative to the kernel virtual filesystem base.
	Start(entryPoint string) error

	// Stop stops the kernel. No further scripts will be executed
	// after this point.
	Stop() error
}
