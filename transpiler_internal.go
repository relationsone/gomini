package gomini

import (
	"github.com/dop251/goja"
	"github.com/apex/log"
	"github.com/go-errors/errors"
	"path/filepath"
	"github.com/spf13/afero"
	"os"
	"fmt"
	"encoding/json"
)

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

func (t *transpiler) __initialize() {
	if t.runtime == nil {
		log.Info("Transpiler: Setting up TypeScript transpiler...")

		t.runtime = goja.New()
		t.runtime.GlobalObject().Set("tsVersion", func(call goja.FunctionCall) goja.Value {
			version := call.Argument(0).String()
			log.Infof("Transpiler: Using bundled TypeScript v%s", version)
			t.transpilerVersion = version
			return goja.Undefined()
		})

		console := t.runtime.NewObject()
		console.Set("log", func(msg interface{}) {
			stackFrames := t.runtime.CaptureCallStack(2)
			frame := stackFrames[1]
			pos := frame.Position()
			log.Infof("%s[%d:%d]: %s", frame.SrcName(), pos.Line, pos.Col, msg)
		})
		t.runtime.GlobalObject().Set("console", console)

		if _, err := t.__loadScript(t.kernel, "/js/typescript"); err != nil {
			panic(err)
		}
		if _, err := t.__loadScript(t.kernel, "/js/tsc"); err != nil {
			panic(err)
		}
	}
}

func (t *transpiler) __transpileSource(source string) (*string, error) {
	// Make sure the underlying runtime is initialized
	t.__initialize()

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

func (t *transpiler) __loadScript(bundle Bundle, filename string) (goja.Value, error) {
	scriptFile := t.kernel.resolveScriptPath(t.kernel, filename)

	loaderFilename := fmt.Sprintf("%s:/%s", scriptFile.loader.Name(), scriptFile.path)
	log.Infof("Transpiler: Cache for '%s' is stale, transpiling now")

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

func (t *transpiler) __findTranspiledModule(filename string) *transpiledModule {
	for _, module := range t.transpilerCache.Modules {
		if module.OriginalFile == filename {
			return &module
		}
	}
	return nil
}

func (t *transpiler) __removeTranspiledModule(module *transpiledModule) error {
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
	if err := t.__storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) __addTranspiledModule(path, cacheFile, code string, bundle Bundle) error {
	module := transpiledModule{path, cacheFile, hash(code), bundle.ID()}
	t.transpilerCache.Modules = append(t.transpilerCache.Modules, module)

	// Store transpiled module information
	if err := t.__storeModuleCacheInformation(); err != nil {
		return err
	}
	return nil
}

func (t *transpiler) __storeModuleCacheInformation() error {
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
