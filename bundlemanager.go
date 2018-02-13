package gomini

import (
	"path/filepath"
	"os"
	"github.com/go-errors/errors"
	"io/ioutil"
	"encoding/json"
	"github.com/spf13/afero"
	"github.com/relationsone/bacc"
	"github.com/apex/log"
)

const (
	appsPath = "/kernel/apps"

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

	return afero.Walk(bm.kernel.filesystem, appsPath, func(path string, info os.FileInfo, err error) error {
		if path == appsPath {
			return nil
		}

		bundle, err := bm.tryLoadBundle(path, info, transpiler)

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

		log.Infof("BundleManager: Loaded bundle %s (%s)", bundle.Name(), bundle.ID())

		if info != nil {
			return filepath.SkipDir
		}
		return nil
	})
}

func (bm *bundleManager) tryLoadBundle(path string, info os.FileInfo, transpiler *transpiler) (bundle Bundle, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = errors.New(r)
			}
		}
	}()

	return bm.loadBundle(path, info, transpiler)
}

func (bm *bundleManager) loadBundle(path string, info os.FileInfo, transpiler *transpiler) (Bundle, error) {
	filesystem, err := bm.createBundleFilesystem(path, info)
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

	return bm.newBundle(path, filesystem, transpiler)
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
	log.Infof("BundleManager: Loading new bundle from kernel:/%s", bundleFile)
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

	bundle.setBundleStatus(BundleStatusStarting)
	_, err = bm.kernel.loadScriptModule(config.Id, config.Name, "/", &resolvedScriptPath{config.Entrypoint, bundle}, bundle)
	if err != nil {
		return nil, err
	}

	bundle.setBundleStatus(BundleStatusStarted)
	return bundle, nil
}

func (bm *bundleManager) registerDefaults(bundle Bundle) error {
	for _, binder := range bm.apiBinders {
		builder := newApiBuilder(nil, bundle, bm.kernel)
		binder(bm.kernel, bundle, builder)
	}

	if _, err := loadPlainJavascript(bm.kernel, jsPromise, bm.kernel, bundle); err != nil {
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
