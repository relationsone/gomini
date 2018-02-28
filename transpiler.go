package gomini

import (
	"path/filepath"
	"os"
	"encoding/json"
	"runtime"
	"github.com/spf13/afero"
	"github.com/apex/log"
)

const cacheJsonFile = "cache.json"
const cacheVfsPath = "/kernel/cache"

type transpiler struct {
	sandbox           Sandbox
	kernel            *kernel
	transpilerVersion string
	transpilerCache   *transpilerCache
}

func newTranspiler(kernel *kernel) (*transpiler, error) {
	var cache *transpilerCache

	cacheFile := filepath.Join(cacheVfsPath, cacheJsonFile)
	if file, err := afero.ReadFile(kernel.Filesystem(), cacheFile); err == nil {
		json.Unmarshal(file, &cache)
	}

	if cache == nil {
		cache = &transpilerCache{
			TranspilerVersion: "",
			Modules:           make([]transpiledModule, 0),
		}
	}

	transpiler := &transpiler{
		sandbox:         nil,
		kernel:          kernel,
		transpilerCache: cache,
	}
	return transpiler, nil
}

func (t *transpiler) transpileFile(bundle Bundle, path string) (*string, error) {
	if !filepath.IsAbs(path) {
		p, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		path = p
	}

	cacheFile := filepath.Join(cacheVfsPath, tsCacheFilename(path, bundle, t.kernel))

	// Load typescript file content
	data, err := t.kernel.loadContent(bundle, bundle.Filesystem(), path)
	if err != nil {
		return nil, err
	}
	code := string(data)

	checksum := hash(code)
	module := t.__findTranspiledModule(path)

	isCached := fileExists(t.kernel.Filesystem(), cacheFile)
	if isCached && module != nil && module.Checksum == checksum {
		log.Debugf("Transpiler: Already transpiled '%s:/%s' as 'kernel:/%s'...", bundle.Name(), path, cacheFile)
		f, err := t.kernel.Filesystem().Open(cacheFile)
		if err != nil {
			return nil, err
		}
		b, err := afero.ReadAll(f)
		if err != nil {
			return nil, err
		}
		source := string(b)
		return &source, nil
	}

	if isCached {
		log.Infof("Transpiler: Cache for '%s:/%s' is stale, transpiling...", bundle.Name(), path)
	}

	// Module exists but either cache file is missing or checksum doesn't match anymore
	// Try to remove old cache file
	t.kernel.filesystem.Remove(cacheFile)

	// Remove old module definition
	t.__removeTranspiledModule(module)

	log.Infof("Transpiler: Transpiling '%s:/%s' to 'kernel:/%s'...", bundle.Name(), path, cacheFile)

	if source, err := t.__transpileSource(code); err != nil {
		return nil, err

	} else {
		if err := afero.WriteFile(t.kernel.Filesystem(), cacheFile, []byte(*source), os.ModePerm); err != nil {
			return nil, err
		}

		if err := t.__addTranspiledModule(path, cacheFile, code, bundle); err != nil {
			return nil, err
		}

		return source, nil
	}
}

func (t *transpiler) transpileAll(bundle Bundle, root string) error {
	if err := afero.Walk(bundle.Filesystem(), root, func(path string, info os.FileInfo, err error) error {
		// Skip directories
		if fi, err := bundle.Filesystem().Stat(path); err != nil {
			return err
		} else {
			if fi.IsDir() {
				return nil
			}
		}

		if isTypeScript(path) {
			if _, err := t.transpileFile(bundle, path); err != nil {
				return err
			}
		}
		return nil

	}); err != nil {
		return err
	}

	// Remove transpiler from memory to free up some space
	if t.sandbox != nil {
		t.sandbox = nil
		runtime.GC()
	}

	return nil
}
