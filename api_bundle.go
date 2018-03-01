package gomini

import (
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
	Export(value Value, target interface{}) error
	Status() BundleStatus
	Filesystem() afero.Fs

	Null() Value
	Undefined() Value

	NewObjectBuilder(objectName string) ObjectCreator
	NewObject() Object
	NewException(err error) Object
	ToValue(value interface{}) Value
	FreezeObject(object Object)
	DeepFreezeObject(object Object)
	NewTypeError(args ...interface{}) Value
	Sandbox() Sandbox

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
