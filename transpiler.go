package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"os"
	"io/ioutil"
	"encoding/json"
	"runtime"
	"github.com/go-errors/errors"
	"taurus/integration/debug"
)

const cacheJsonFile = "cache.json"

type transpiler struct {
	vm                *goja.Runtime
	kernel            *kernel
	transpiledModules []transpiledModule
}

func newTranspiler(kernel *kernel) (*transpiler, error) {
	var modules []transpiledModule

	cacheFile := filepath.Join(kernel.transpilerCacheDir, cacheJsonFile)
	if file, err := ioutil.ReadFile(cacheFile); err == nil {
		json.Unmarshal(file, &modules)
	}

	if modules == nil {
		modules = make([]transpiledModule, 0)
	}

	if !fileExists(kernel.transpilerCacheDir) {
		os.Mkdir(kernel.transpilerCacheDir, os.ModePerm)
	}

	transpiler := &transpiler{
		vm:                nil,
		kernel:            kernel,
		transpiledModules: modules,
	}
	return transpiler, nil
}

func (t *transpiler) initialize() {
	if t.vm == nil {
		debug.DebugLog("Setting up typescript transpiler...")

		t.vm = goja.New()
		if _, err := t.loadScript("js/typescript"); err != nil {
			panic(err)
		}
		if _, err := t.loadScript("js/tsc"); err != nil {
			panic(err)
		}
	}
}

func (t *transpiler) transpileFile(path string) (*string, error) {
	if !filepath.IsAbs(path) {
		p, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		path = p
	}

	filename := hash(path)
	cacheFile := filepath.Join(t.kernel.transpilerCacheDir, filename)

	// Load typescript file content
	data, err := t.kernel.loadContent(path)
	if err != nil {
		return nil, err
	}
	code := string(data)

	checksum := hash(code)
	module := t.findTranspiledModule(path)

	if fileExists(cacheFile) && module != nil && module.Checksum == checksum {
		debug.DebugLog("Already transpiled %s as %s...", path, cacheFile)
		return nil, nil
	}

	// Module exists but either cache file is missing or checksum doesn't match anymore
	// Try to remove old cache file
	os.Remove(cacheFile)

	// Remove old module definition
	t.removeTranspiledModule(module)

	debug.DebugLog("Transpiling %s to %s...", path, cacheFile)

	if source, err := t._transpileSource(code); err != nil {
		return nil, err

	} else {
		if err := ioutil.WriteFile(cacheFile, []byte(*source), os.ModePerm); err != nil {
			return nil, err
		}

		if err := t.addTranspiledModule(path, cacheFile, code); err != nil {
			return nil, err
		}

		return source, nil
	}
}

func (t *transpiler) _transpileSource(source string) (*string, error) {
	// Make sure the underlying runtime is initialized
	t.initialize()

	// Retrieve the transpiler function from the runtime
	jsTranspiler := t.vm.Get("transpiler")
	if jsTranspiler == nil || jsTranspiler == goja.Null() {
		panic(errors.New("transpiler function not available"))
	}

	var transpiler goja.Callable
	if err := t.vm.ExportTo(jsTranspiler, &transpiler); err != nil {
		return nil, err
	}

	// Transpile
	if val, err := transpiler(jsTranspiler, t.vm.ToValue(source)); err != nil {
		return nil, err
	} else {
		source := val.String()
		return &source, nil
	}
}

func (t *transpiler) transpileAll() error {
	if baseDir, err := filepath.Abs(t.kernel.basePath); err != nil {
		return err

	} else {
		tsDir := filepath.Clean(baseDir)

		if !fileExists(t.kernel.transpilerCacheDir) {
			os.Mkdir(t.kernel.transpilerCacheDir, os.ModePerm)
		}

		if err := filepath.Walk(tsDir, func(path string, info os.FileInfo, err error) error {
			// Skip directories
			if fi, err := os.Stat(path); err != nil {
				return err
			} else {
				if fi.IsDir() {
					return nil
				}
			}

			if isTypeScript(path) {
				if _, err := t.transpileFile(path); err != nil {
					return err
				}
			}
			return nil

		}); err != nil {
			return err
		}
	}

	// Remove transpiler from memory to free up some space
	if t.vm != nil {
		t.vm = nil
		runtime.GC()
	}

	return nil
}

func (t *transpiler) loadScript(filename string) (goja.Value, error) {
	scriptFile := findScriptFile(filename, t.kernel.basePath)
	scriptFile, err := filepath.Abs(scriptFile)
	if err != nil {
		return nil, err
	}
	if source, err := t.kernel.loadContent(scriptFile); err != nil {
		return nil, err

	} else {
		return prepareJavascript(scriptFile, string(source), t.vm)
	}
}

func (t *transpiler) findTranspiledModule(filename string) *transpiledModule {
	for _, module := range t.transpiledModules {
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

	for i, temp := range t.transpiledModules {
		if temp.OriginalFile == module.OriginalFile {
			t.transpiledModules = append(t.transpiledModules[:i], t.transpiledModules[i+1:]...)
			return nil
		}
	}

	// Store transpiled module information
	if err := t.storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) addTranspiledModule(path, cacheFile, code string) error {
	module := transpiledModule{path, cacheFile, hash(code)}
	t.transpiledModules = append(t.transpiledModules, module)

	// Store transpiled module information
	if err := t.storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) storeModuleCacheInformation() error {
	file := filepath.Join(t.kernel.transpilerCacheDir, cacheJsonFile)
	os.Remove(file)
	if data, err := json.Marshal(t.transpiledModules); err != nil {
		return err
	} else {
		ioutil.WriteFile(file, data, os.ModePerm)
	}
	return nil
}

type transpiledModule struct {
	OriginalFile string `json:"originalFile"`
	CacheFile    string `json:"cache_file"`
	Checksum     string `json:"checksum"`
}
