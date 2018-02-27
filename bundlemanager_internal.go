package gomini

import (
	"os"
	"path/filepath"
	"github.com/go-errors/errors"
	"github.com/spf13/afero"
	"github.com/relationsone/bacc"
	"io/ioutil"
	"encoding/json"
	"github.com/apex/log"
)

type bundleConfig struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	Entrypoint string   `json:"entrypoint"`
	Privileges []string `json:"privileges"`
}

func (bm *bundleManager) __createBundleFilesystem(path string, info os.FileInfo) (*CompositeFs, error) {
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
		exportfs := newKernelFs()
		// TODO Add privilege checks to only make requested modules available
		root := exportfs.root
		for _, m := range bm.kernel.modules {
			if m.kernel {
				if err := root.createFile(m.origin.Filename(), []byte{}, bm.__bindModuleToKernelSyscall(m)); err != nil {
					return nil, err
				}
			}
		}
		compositefs.Mount(exportfs, "/kernel/@types/")

		return compositefs, nil
	}

	return nil, errNoSuchBundle
}

func (bm *bundleManager) __bindModuleToKernelSyscall(module Module) KernelSyscall {
	return func(caller Bundle) Any {
		return module
	}
}

func (bm *bundleManager) __newBundle(path string, bundlefs afero.Fs, transpiler *transpiler) (Bundle, error) {
	bundleFile := filepath.Join(path, bundleJson)
	log.Infof("BundleManager: Loading new bundle from kernel:/%s", bundleFile)
	reader, err := bundlefs.Open(bundleJson)
	if err != nil {
		return nil, errors.New(err)
	}
	defer reader.Close()

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

	bundle.setBundleStatus(BundleStatusStarting)
	_, err = bm.kernel.loadScriptModule(config.Id, config.Name, "/", &resolvedScriptPath{config.Entrypoint, bundle}, bundle)
	if err != nil {
		return nil, err
	}

	bundle.setBundleStatus(BundleStatusStarted)
	return bundle, nil
}

func (bm *bundleManager) __tryLoadBundle(path string, info os.FileInfo, transpiler *transpiler) (bundle Bundle, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = errors.New(r)
			}
		}
	}()

	return bm.__loadBundle(path, info, transpiler)
}

func (bm *bundleManager) __loadBundle(path string, info os.FileInfo, transpiler *transpiler) (Bundle, error) {
	filesystem, err := bm.__createBundleFilesystem(path, info)
	if err == errNoSuchBundle {
		if info.IsDir() {
			return nil, filepath.SkipDir
		} else {
			return nil, nil
		}
	}
	if err != nil {
		return nil, err
	}

	return bm.__newBundle(path, filesystem, transpiler)
}
