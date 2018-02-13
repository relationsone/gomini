package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"os"
	"encoding/json"
	"runtime"
	"github.com/go-errors/errors"
	"fmt"
	"github.com/spf13/afero"
	"github.com/apex/log"
)

const cacheJsonFile = "cache.json"
const cacheVfsPath = "/kernel/cache"

type transpiler struct {
	runtime           *goja.Runtime
	kernel            *kernel
	transpilerVersion string
	transpilerCache   *transpilerCache
}

func newTranspiler(kernel *kernel) (*transpiler, error) {
	var cache *transpilerCache

	cacheFile := filepath.Join(cacheVfsPath, cacheJsonFile)
	if file, err := afero.ReadFile(kernel.Filesystem(), cacheFile); err == nil {
		json.Unmarshal(file, cache)
	}

	if cache == nil {
		cache = &transpilerCache{
			TranspilerVersion: "",
			Modules:           make([]transpiledModule, 0),
		}
	}

	transpiler := &transpiler{
		runtime:         nil,
		kernel:          kernel,
		transpilerCache: cache,
	}
	return transpiler, nil
}

func (t *transpiler) initialize() {
	if t.runtime == nil {
		log.Info("Transpiler: Setting up TypeScript transpiler...")

		t.runtime = goja.New()
		t.runtime.GlobalObject().Set("tsVersion", func(call goja.FunctionCall) goja.Value {
			version := call.Argument(0).String()
			log.Infof("Transpiler: Using bundled TypeScript compiler V%s", version)
			t.transpilerVersion = version
			return goja.Undefined()
		})

		if _, err := t.loadScript(t.kernel, "/js/typescript"); err != nil {
			panic(err)
		}
		if _, err := t.loadScript(t.kernel, "/js/tsc"); err != nil {
			panic(err)
		}
	}
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
	module := t.findTranspiledModule(path)

	isCached := fileExists(t.kernel.Filesystem(), cacheFile)
	if isCached && module != nil && module.Checksum == checksum {
		log.Infof("Transpiler: Already transpiled %s:/%s as kernel:/%s...", bundle.Name(), path, cacheFile)
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
		log.Infof("Transpiler: Cache is out of date for %s:/%s as kernel:/%s...", bundle.Name(), path, cacheFile)
	}

	// Module exists but either cache file is missing or checksum doesn't match anymore
	// Try to remove old cache file
	t.kernel.filesystem.Remove(cacheFile)

	// Remove old module definition
	t.removeTranspiledModule(module)

	log.Infof("Transpiler: Transpiling %s:/%s to kernel:/%s...", bundle.Name(), path, cacheFile)

	if source, err := t._transpileSource(code); err != nil {
		return nil, err

	} else {
		if err := afero.WriteFile(t.kernel.Filesystem(), cacheFile, []byte(*source), os.ModePerm); err != nil {
			return nil, err
		}

		if err := t.addTranspiledModule(path, cacheFile, code, bundle); err != nil {
			return nil, err
		}

		return source, nil
	}
}

func (t *transpiler) _transpileSource(source string) (*string, error) {
	// Make sure the underlying runtime is initialized
	t.initialize()

	// Retrieve the transpiler function from the runtime
	jsTranspiler := t.runtime.Get("transpiler")
	if jsTranspiler == nil || jsTranspiler == goja.Null() {
		panic(errors.New("transpiler function not available"))
	}

	var transpiler goja.Callable
	if err := t.runtime.ExportTo(jsTranspiler, &transpiler); err != nil {
		return nil, err
	}

	// Transpile
	if val, err := transpiler(jsTranspiler, t.runtime.ToValue(source)); err != nil {
		return nil, err
	} else {
		source := val.String()
		return &source, nil
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
	if t.runtime != nil {
		t.runtime = nil
		runtime.GC()
	}

	return nil
}

func (t *transpiler) loadScript(bundle Bundle, filename string) (goja.Value, error) {
	scriptFile := t.kernel.resolveScriptPath(t.kernel, filename)

	loaderFilename := fmt.Sprintf("%s:/%s", scriptFile.loader, scriptFile.path)

	source, err := t.kernel.loadContent(bundle, scriptFile.loader.Filesystem(), scriptFile.path)
	if err != nil {
		return nil, err

	}

	prog, err := compileJavascript(loaderFilename, string(source))
	if err != nil {
		return nil, err
	}

	return t.runtime.RunProgram(prog)
}

func (t *transpiler) findTranspiledModule(filename string) *transpiledModule {
	for _, module := range t.transpilerCache.Modules {
		if module.OriginalFile == filename {
			return &module
		}
	}
	return nil
}

func (t *transpiler) removeTranspiledModule(module *transpiledModule) error {
	if module == nil {
		return nil
	}

	for i, temp := range t.transpilerCache.Modules {
		if temp.OriginalFile == module.OriginalFile {
			t.transpilerCache.Modules = append(t.transpilerCache.Modules[:i], t.transpilerCache.Modules[i+1:]...)
			return nil
		}
	}

	// Store transpiled module information
	if err := t.storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) addTranspiledModule(path, cacheFile, code string, bundle Bundle) error {
	module := transpiledModule{path, cacheFile, hash(code), bundle.ID()}
	t.transpilerCache.Modules = append(t.transpilerCache.Modules, module)

	// Store transpiled module information
	if err := t.storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) storeModuleCacheInformation() error {
	file := filepath.Join(cacheVfsPath, cacheJsonFile)
	t.kernel.filesystem.Remove(file)
	if data, err := json.Marshal(t.transpilerCache); err != nil {
		return err
	} else {
		if err := afero.WriteFile(t.kernel.filesystem, file, data, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

type transpilerCache struct {
	TranspilerVersion string             `json:"transpiler_version"`
	Modules           []transpiledModule `json:"modules"`
}

type transpiledModule struct {
	OriginalFile string `json:"original_file"`
	CacheFile    string `json:"cache_file"`
	Checksum     string `json:"checksum"`
	BundleId     string `json:"bundle_id"`
}
