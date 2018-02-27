package gomini

import (
	"github.com/dop251/goja"
	"github.com/spf13/afero"
)

type BundleStatus int

const (
	BundleStatusStopped     BundleStatus = iota
	BundleStatusStarted
	BundleStatusStarting
	BundleStatusStopping
	BundleStatusDownloading
	BundleStatusUpdating
	BundleStatusFailed
	BundleStatusInstalled
)

func (b BundleStatus) String() string {
	switch b {
	case BundleStatusInstalled:
		return "INSTALLED"
	case BundleStatusStarting:
		return "STARTING"
	case BundleStatusStarted:
		return "STARTED"
	case BundleStatusStopping:
		return "STOPPING"
	case BundleStatusStopped:
		return "STOPPED"
	case BundleStatusDownloading:
		return "DOWNLOADING"
	case BundleStatusUpdating:
		return "UPDATING"
	case BundleStatusFailed:
		return "FAILED"
	}
	panic("illegal bundle status")
}

type Bundle interface {
	ID() string
	Name() string
	Privileged() bool
	Privileges() []string
	SecurityInterceptor() SecurityInterceptor
	Export(value goja.Value, target Any) error
	Status() BundleStatus
	Filesystem() afero.Fs

	Null() JsValue
	Undefined() JsValue

	NewObjectBuilder(objectName string) BundleObjectBuilder
	NewObject() *goja.Object
	NewException(err error) *goja.Object
	ToValue(value Any) JsValue
	FreezeObject(object *goja.Object)
	DeepFreezeObject(object *goja.Object)
	NewTypeError(args ...Any) JsValue
	Sandbox() *goja.Runtime

	getSecurityProxy() *securityProxy
	findModuleById(id string) *module
	findModuleByName(name string) *module
	findModuleByModuleFile(file string) *module
	addModule(module *module)
	removeModule(module *module)
	peekLoaderStack() string
	popLoaderStack() string
	pushLoaderStack(element string)
	getBasePath() string
	setBundleStatus(status BundleStatus)
}

type BundleObjectBuilder interface {
	JsObjectBuilder
	Build() JsObject
}
