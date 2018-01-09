package gomini

import "github.com/dop251/goja"

type bundleManager struct {
	baseDir string
}

func newBundleManager(baseDir string) *bundleManager {
	return &bundleManager{
		baseDir: baseDir,
	}
}

func (bm *bundleManager) newBundle(bundlePath string) Bundle {
	vm := goja.New()

}