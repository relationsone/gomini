package gomini

import (
	"path/filepath"
	"os"
	"github.com/go-errors/errors"
	"github.com/spf13/afero"
	"github.com/apex/log"
)

const (
	bundleJson = "bundle.json"
	jsPromise  = "/js/kernel/promise.js"
)

var errNoSuchBundle = errors.New("the given path is not a bundle")

func newBundleManager(kernel *kernel, apiBinders []ApiProviderBinder) *bundleManager {
	return &bundleManager{
		kernel:     kernel,
		apiBinders: apiBinders,
	}
}

type bundleManager struct {
	kernel     *kernel
	apiBinders []ApiProviderBinder
}

func (bm *bundleManager) start() error {
	transpiler, err := newTranspiler(bm.kernel)
	if err != nil {
		return err
	}

	return afero.Walk(bm.kernel.filesystem, KernelVfsAppsPath, func(path string, info os.FileInfo, err error) error {
		if path == KernelVfsAppsPath {
			return nil
		}

		bundle, err := bm.__tryLoadBundle(path, info, transpiler)

		if err != nil {
			if err != filepath.SkipDir {
				log.Warnf("BundleManager: Loading bundle from path %s failed: %s", path, err.Error())
				return nil
			}
			return err
		}

		if bundle == nil {
			return nil
		}

		log.Infof("BundleManager: Loaded bundle %s", bundle.Name())

		if info != nil {
			return filepath.SkipDir
		}
		return nil
	})
}

func (bm *bundleManager) stop() error {
	// TODO: bundleManager needs to be able to be stopped
	return nil
}

func (bm *bundleManager) registerDefaults(bundle Bundle) error {
	for _, binder := range bm.apiBinders {
		objectBuilder := bundle.Sandbox().NewObjectCreator("")
		binder(bm.kernel, bundle, objectBuilder)
		objectBuilder.BuildInto("", bundle.Sandbox().Global())
	}

	if _, err := loadPlainJavascript(bm.kernel, jsPromise, bm.kernel, bundle); err != nil {
		return err
	}

	return nil
}
