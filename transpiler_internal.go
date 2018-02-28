package gomini

import (
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
	if t.sandbox == nil {
		log.Info("Transpiler: Setting up TypeScript transpiler...")

		t.sandbox = t.kernel.sandboxFactory(t.kernel)
		t.sandbox.Global().DefineFunction("tsVersion", "tsVersion", func(call FunctionCall) Value {
			version := call.Argument(0).String()
			log.Infof("Transpiler: Using bundled TypeScript v%s", version)
			t.transpilerVersion = version
			return t.sandbox.UndefinedValue()
		})

		builder := t.sandbox.NewObjectCreator("console")
		builder.DefineGoFunction("log", "log", func(msg Any) {
			stackFrames := t.sandbox.CaptureCallStack(2)
			frame := stackFrames[1]
			pos := frame.Position()
			log.Infof("%s[%d:%d]: %s", frame.SrcName(), pos.Line, pos.Col, msg)
		})
		builder.BuildInto("console", t.sandbox.Global())

		if _, err := t.__loadScript(t.kernel, "/js/typescript", ""); err != nil {
			panic(err)
		}
		if _, err := t.__loadScript(t.kernel, "embedded://tsc.js", tscSource); err != nil {
			panic(err)
		}
	}
}

func (t *transpiler) __transpileSource(source string) (*string, error) {
	// Make sure the underlying runtime is initialized
	t.__initialize()

	// Retrieve the transpiler function from the runtime
	jsTranspiler := t.sandbox.Global().Get("transpiler")
	if jsTranspiler == nil || jsTranspiler == t.sandbox.NullValue() {
		panic(errors.New("transpiler function not available"))
	}

	var transpiler Callable
	if err := t.sandbox.Export(jsTranspiler, &transpiler); err != nil {
		return nil, err
	}

	// Transpile
	if val, err := transpiler(jsTranspiler, t.sandbox.ToValue(source)); err != nil {
		return nil, err
	} else {
		source := val.String()
		return &source, nil
	}
}

func (t *transpiler) __loadScript(bundle Bundle, filename string, source string) (Value, error) {
	if source == "" {
		scriptFile := t.kernel.resolveScriptPath(t.kernel, filename)

		filename = fmt.Sprintf("%s:/%s", scriptFile.loader.Name(), scriptFile.path)

		s, err := t.kernel.loadContent(bundle, scriptFile.loader.Filesystem(), scriptFile.path)
		if err != nil {
			return nil, err

		}
		source = string(s)
	}

	script, _, err := t.sandbox.Compile(filename, source)
	if err != nil {
		return nil, err
	}

	return t.sandbox.Execute(script)
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

const tscSource = `
tsVersion(ts.version);

function transpiler(source) {
    var result = ts.transpileModule(source, {
        compilerOptions: {
            moduleResolution: "node",
            module: "System",
            target: "es5",
            isolatedModules: true,
            importHelpers: true,
            tsconfig: false,
            noImplicitAny: false,
            alwaysStrict: true,
            inlineSourceMap: true,
            diagnostics: true,
            strictPropertyInitialization: true,
            allowJs: false,
            downlevelIteration: true,
            noLib: true,
            declaration: true,
            typeRoots: [
                "scripts/types"
            ],
            lib: [
                "lib/libbase.d.ts"
            ]
        },
        reportDiagnostics: true,
        transformers: []
    });

    for (var i = 0; i < result.diagnostics.length; i++) {
        ts.sys.write(result.diagnostics[i]);
    }

    return result.outputText;
}
`
