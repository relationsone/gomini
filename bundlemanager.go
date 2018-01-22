package gomini

import (
	"github.com/dop251/goja"
	"fmt"
	"path/filepath"
	"os"
	"github.com/go-errors/errors"
	"io/ioutil"
	"encoding/json"
	"github.com/spf13/afero"
	"github.com/relationsone/bacc"
)

const (
	appsPath = "/kernel/apps"

	bundleJson = "bundle.json"
	jsPromise  = "/js/kernel/promise.js"
	jsSystem   = "/js/kernel/system.js"
)

type bundlePrivilege int16

func (b bundlePrivilege) ToString() (name string) {
	switch b {
	case privilegeKernel:
		name = "PRIVILEGE_KERNEL"

	case privilegeDatabase:
		name = "PRIVILEGE_DATABASE"
	case privilegeEncoding:
		name = "PRIVILEGE_ENCODING"
	case privilegeGenerate:
		name = "PRIVILEGE_GENERATE"
	case privilegeHttp:
		name = "PRIVILEGE_HTTP"

	case privilegeConfigStorage:
		name = "PRIVILEGE_CONFIG_STORAGE"
	}
	return
}

const (
	privilegeKernel bundlePrivilege = iota

	privilegeDatabase
	privilegeEncoding
	privilegeGenerate
	privilegeHttp

	privilegeConfigStorage
)

var errNoSuchBundle = errors.New("the given path is not a bundle")

func newBundleManager(kernel *kernel) *bundleManager {
	return &bundleManager{
		kernel: kernel,
	}
}

type bundleManager struct {
	kernel *kernel
}

func (bm *bundleManager) start() error {
	return afero.Walk(bm.kernel.filesystem, appsPath, func(path string, info os.FileInfo, err error) error {
		if path == appsPath {
			return nil
		}
		if info != nil {
			filesystem, err := bm.createBundleFilesystem(path, info)
			if err == errNoSuchBundle {
				if info.IsDir() {
					return filepath.SkipDir
				} else {
					return nil
				}
			}
			if err != nil {
				return err
			}

			bundle, err := bm.newBundle(path, filesystem)
			if err != nil {
				return err
			}

			fmt.Println(fmt.Sprintf("BundleManager: Loaded bundle %s (%s)", bundle.Name(), bundle.ID()))
			return filepath.SkipDir
		}
		return nil
	})
}

func (bm *bundleManager) createBundleFilesystem(path string, info os.FileInfo) (*CompositeFs, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		rofs := afero.NewReadOnlyFs(bm.kernel.filesystem)
		rootfs := afero.NewBasePathFs(rofs, path)
		return NewCompositeFs(rootfs), nil
	}

	if filepath.Ext(path) == "bacc" {
		bacc.NewBaccFilesystem(path, bm.kernel.keyManager)
	}

	return nil, errNoSuchBundle
}

func (bm *bundleManager) newBundle(path string, bundlefs afero.Fs) (Bundle, error) {
	bundleFile := filepath.Join(path, bundleJson)
	fmt.Println(fmt.Sprintf("Loading new bundle from %s", bundleFile))
	reader, err := bundlefs.Open(bundleJson)
	if err != nil {
		return nil, errors.New(err)
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.New(err)
	}

	config := bundleConfig{}
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, errors.New(err)
	}

	bundle, err := newBundle(bm.kernel, bundlefs, config.Id, config.Name)
	if err != nil {
		return nil, err
	}

	bundle.init(bm.kernel)
	bm.kernel.loadScriptModule(config.Id, config.Name, config.Entrypoint, "/", bundle)

	return bundle, nil
}

func (bm *bundleManager) registerDefaults(bundle Bundle) error {
	console := bundle.getSandbox().NewObject()
	if err := console.Set("log", func(msg string) {
		fmt.Println(msg)

	}); err != nil {
		return err
	}

	bundle.getSandbox().Set("console", console)
	bundle.getSandbox().Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		return goja.Null()
	})

	if _, err := loadPlainJavascript(bm.kernel, jsPromise, bundle); err != nil {
		return err
	}

	if _, err := loadPlainJavascript(bm.kernel, jsSystem, bundle); err != nil {
		return err
	}

	return nil
}

type bundleConfig struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Entrypoint string   `json:"entrypoint"`
	Privileges []string `json:"privileges"`
}
