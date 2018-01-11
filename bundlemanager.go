package gomini

import (
	"github.com/dop251/goja"
	"fmt"
	"path/filepath"
	"os"
	"github.com/go-errors/errors"
	"io/ioutil"
	"encoding/json"
)

const (
	bundleJson = "bundle.json"
	jsPromise  = "js/kernel/promise.js"
	jsSystem   = "js/kernel/system.js"
)

const (
	privilegeKernel   = "PRIVILEGE_KERNEL"
	privilegeDatabase = "PRIVILEGE_DATABASE"
)

func newBundleManager(kernel *kernel, baseDir string) *bundleManager {
	return &bundleManager{
		kernel:  kernel,
		baseDir: baseDir,
	}
}

type bundleManager struct {
	kernel  *kernel
	baseDir string
}

func (bm *bundleManager) newBundle(bundlePath string) (Bundle, error) {
	bundleFile := filepath.Join(bundlePath, bundleJson)
	reader, err := os.Open(bundleFile)
	if err != nil {
		return nil, errors.New(err)
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.New(err)
	}

	var config bundleConfig
	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, errors.New(err)
	}

	return newBundle(bm.kernel, config.id, config.name, bundlePath)
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

	if _, err := loadPlainJavascript(bm.kernel, jsPromise, bm.baseDir, bundle.getSandbox()); err != nil {
		return err
	}

	if _, err := loadPlainJavascript(bm.kernel, jsSystem, bm.baseDir, bundle.getSandbox()); err != nil {
		return err
	}

	return nil
}

type bundleConfig struct {
	id         string   `json:"id"`
	name       string   `json:"name"`
	entrypoint string   `json:"entrypoint"`
	privileges []string `json:"privileges"`
}
