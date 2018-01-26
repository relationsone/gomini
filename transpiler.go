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
)

const cacheJsonFile = "cache.json"
const cacheVfsPath = "/kernel/cache"

type transpiler struct {
	runtime           *goja.Runtime
	kernel            *kernel
	transpiledModules []transpiledModule
}

func newTranspiler(kernel *kernel) (*transpiler, error) {
	var modules []transpiledModule

	// TODO cacheFile := filepath.Join(kernel.transpilerCachePath, cacheJsonFile)
	cacheFile := filepath.Join(cacheVfsPath, cacheJsonFile)
	if file, err := afero.ReadFile(kernel.Filesystem(), cacheFile); err == nil {
		json.Unmarshal(file, &modules)
	}

	if modules == nil {
		modules = make([]transpiledModule, 0)
	}

	/* TODO if !fileExists(kernel.osfs, cacheVfsPath) {
		os.Mkdir(cacheVfsPath, os.ModePerm)
	}*/

	transpiler := &transpiler{
		runtime:           nil,
		kernel:            kernel,
		transpiledModules: modules,
	}
	return transpiler, nil
}

func (t *transpiler) initialize() {
	if t.runtime == nil {
		fmt.Println("Transpiler: Setting up typescript transpiler...")

		t.runtime = goja.New()
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

	filename := hash(path)
	// TODO cacheFile := filepath.Join(t.kernel.transpilerCachePath, filename)
	cacheFile := filepath.Join(cacheVfsPath, filename)

	// Load typescript file content
	data, err := t.kernel.loadContent(bundle, bundle.Filesystem(), path)
	if err != nil {
		return nil, err
	}
	code := string(data)

	checksum := hash(code)
	module := t.findTranspiledModule(path)

	if fileExists(t.kernel.Filesystem(), cacheFile) && module != nil && module.Checksum == checksum {
		fmt.Println(fmt.Sprintf("Transpiler: Already transpiled %s as %s...", path, cacheFile))
		return nil, nil
	}

	// Module exists but either cache file is missing or checksum doesn't match anymore
	// Try to remove old cache file
	t.kernel.filesystem.Remove(cacheFile)

	// Remove old module definition
	t.removeTranspiledModule(module)

	fmt.Println(fmt.Sprintf("Transpiler: Transpiling %s to %s...", path, cacheFile))

	if source, err := t._transpileSource(code); err != nil {
		return nil, err

	} else {
		if err := afero.WriteFile(t.kernel.Filesystem(), cacheFile, []byte(*source), os.ModePerm); err != nil {
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

func (t *transpiler) transpileAll() error {
	/* TODO if !fileExists(t.kernel.osfs, t.kernel.transpilerCachePath) {
		os.Mkdir(t.kernel.transpilerCachePath, os.ModePerm)
	}*/

	if err := afero.Walk(t.kernel.filesystem, "/", func(path string, info os.FileInfo, err error) error {
		// Skip directories
		if fi, err := t.kernel.filesystem.Stat(path); err != nil {
			return err
		} else {
			if fi.IsDir() {
				return nil
			}
		}

		if isTypeScript(path) {
			if _, err := t.transpileFile(t.kernel, path); err != nil {
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
	scriptFile := t.kernel.findScriptFile(t.kernel, filename)
	scriptFile, err := filepath.Abs(scriptFile)
	if err != nil {
		return nil, err
	}
	source, err := t.kernel.loadContent(bundle, bundle.Filesystem(), scriptFile)
	if err != nil {
		return nil, err

	}

	prog, err := compileJavascript(scriptFile, string(source))
	if err != nil {
		return nil, err
	}

	return t.runtime.RunProgram(prog)
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
	// TODO file := filepath.Join(t.kernel.transpilerCachePath, cacheJsonFile)
	file := filepath.Join(cacheVfsPath, cacheJsonFile)
	t.kernel.filesystem.Remove(file)
	if data, err := json.Marshal(t.transpiledModules); err != nil {
		return err
	} else {
		if err := afero.WriteFile(t.kernel.filesystem, file, data, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

type transpiledModule struct {
	OriginalFile string `json:"originalFile"`
	CacheFile    string `json:"cache_file"`
	Checksum     string `json:"checksum"`
}
