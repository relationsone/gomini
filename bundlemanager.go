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
	transpiler, err := newTranspiler(bm.kernel)
	if err != nil {
		return err
	}

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

			bundle, err := bm.newBundle(path, filesystem, transpiler)
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

	var compositefs *CompositeFs = nil
	if info.IsDir() {
		rofs := afero.NewReadOnlyFs(bm.kernel.filesystem)
		rootfs := afero.NewBasePathFs(rofs, path)
		compositefs = NewCompositeFs(rootfs)
	}

	if filepath.Ext(path) == "bacc" {
		rootfs, err := bacc.NewBaccFilesystem(path, bm.kernel.keyManager)
		if err != nil {
			return nil, err
		}
		compositefs = NewCompositeFs(rootfs)
	}

	if compositefs != nil {
		exportfs := newExportFs()
		// TODO Add privilege checks to only make requested modules available
		root := exportfs.root
		for _, m := range bm.kernel.modules {
			if m.kernel {
				if err := root.createFile(m.origin.Filename(), []byte{}, m); err != nil {
					return nil, err
				}
			}
		}
		compositefs.Mount(exportfs, "/kernel/@types/")

		return compositefs, nil
	}

	return nil, errNoSuchBundle
}

func (bm *bundleManager) newBundle(path string, bundlefs afero.Fs, transpiler *transpiler) (Bundle, error) {
	bundleFile := filepath.Join(path, bundleJson)
	fmt.Println(fmt.Sprintf("BundleManager: Loading new bundle from kernel:/%s", bundleFile))
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

	bundle, err := newBundle(bm.kernel, path, bundlefs, config.Id, config.Name, config.Privileges)
	if err != nil {
		return nil, err
	}

	bundle.init(bm.kernel)

	// make sure everything is nicely pretranspiled
	// transpiler.transpileAll(bundle, "/")

	_, err = bm.kernel.loadScriptModule(config.Id, config.Name, config.Entrypoint, "/", bundle)
	if err != nil {
		return nil, err
	}

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
