package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"fmt"
	"strings"
	"github.com/go-errors/errors"
	"io/ioutil"
	"compress/gzip"
	"compress/bzip2"
	"bytes"
	"github.com/satori/go.uuid"
)

const kernel_id = "76141a6c-0aec-4973-b04b-8fdd54753e03"

type registerExport func(name string, value goja.Value)
type registerCallback func(export registerExport, context *goja.Object) *goja.Object

/**
 * The kernel is a special bundle type, which is the root bundle to be initialized and has
 * all privileges (PRIVILEGE_KERNEL) and can leave bundle boundaries.
 */
type kernel struct {
	*bundle
	bundleManager      *bundleManager
	transpilerCacheDir string
	kernelDebugging    bool
}

func NewScriptKernel(basePath, transpilerCacheDir string, kernelDebugging bool) (*kernel, error) {
	if !filepath.IsAbs(basePath) {
		absBaseDir, err := filepath.Abs(basePath)
		if err != nil {
			panic(err)
		} else {
			basePath = absBaseDir
		}
	}

	if !strings.HasSuffix(basePath, "/") {
		basePath = basePath + "/"
	}

	if !filepath.IsAbs(transpilerCacheDir) {
		d, err := filepath.Abs(transpilerCacheDir)
		if err != nil {
			return nil, errors.New(err)
		}
		transpilerCacheDir = d
	}

	if !strings.HasSuffix(transpilerCacheDir, "/") {
		transpilerCacheDir = transpilerCacheDir + "/"
	}

	kernel := &kernel{
		transpilerCacheDir: transpilerCacheDir,
		kernelDebugging:    kernelDebugging,
	}

	kernel.bundleManager = newBundleManager(kernel, basePath)
	bundle, err := newBundle(kernel, kernel_id, "kernel", basePath)
	if err != nil {
		return nil, err
	}
	kernel.bundle = bundle

	// Pre-transpile all typescript sourcefiles that are out of date
	if transpiler, err := newTranspiler(kernel); err != nil {
		return nil, errors.New(err)
	} else {
		if err := transpiler.transpileAll(); err != nil {
			return nil, errors.New(err)
		}
	}

	return kernel, nil
}

func (k *kernel) Privileged() bool {
	return true
}

func (k *kernel) SecurityInterceptor() SecurityInterceptor {
	return func(caller Bundle, property string) (accessGranted bool) {
		if caller.Privileged() {
			return true
		}

		//TODO: add real checks here
		return true
	}
}

func (k *kernel) LoadKernelModule(kernelModule KernelModuleDefinition) error {
	filename := findScriptFile(kernelModule.ApiDefinitionFile(), k.basePath)

	origin := newOrigin(filename)
	module, err := newModule(kernelModule.ID(), kernelModule.Name(), origin, k)
	if err != nil {
		return err
	}
	k.addModule(module)

	moduleBuilder := newModuleBuilder(module, k)
	binder := kernelModule.ExtensionBinder()
	binder(k, moduleBuilder)

	return nil
}

func (k *kernel) EntryPoint(filename string) error {
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	_, err = k.loadScriptModule(id.String(), "entrypoint", filename, k.basePath, k.bundle)
	if err != nil {
		return err
	}

	return nil
}

func (k *kernel) defineKernelModule(module Module, filename string, exporter func(exports *goja.Object)) {
	// Load the script definition file
	_, err := k.loadScriptModule(module.ID(), module.Name(), filename, k.basePath, k.bundle)
	if err != nil {
		panic(errors.New(err))
	}

	// Override goExports
	exporter(module.getModuleExports())

	// Freeze module
	module.Bundle().FreezeObject(module.getModuleExports())
}

func (k *kernel) loadSource(filename string) (string, error) {
	if isTypeScript(filename) {
		// Is pre-transpiled?
		cacheFilename := filepath.Join(k.transpilerCacheDir, hash(filename))
		if !fileExists(cacheFilename) {
			if k.kernelDebugging {
				fmt.Println(fmt.Sprintf("Loading scriptfile '%s' with live transpiler", filename))
			}

			source, err := k.transpile(filename)
			if err != nil {
				return "", err
			}

			// DEBUG
			if k.kernelDebugging {
				fmt.Println(*source)
			}
			return *source, nil
		}

		// Override filename with the pre-transpiled, cached file
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Loading scriptfile '%s' from pretranspiled cache: %s", filename, cacheFilename))
		}

		filename = cacheFilename
	}
	if data, err := k.loadContent(filename); err != nil {
		return "", err
	} else {
		return string(data), nil
	}
}

func (k *kernel) isolateSystemObject(bundle Bundle, source string, isolate bool) ([]byte, error) {
	if !isolate {
		return []byte(source), nil
	}

	return []byte(fmt.Sprintf("(function(System) {\n%s\n})()", source)), nil
}

func (k *kernel) generateRandomIdentifier(prefix string) (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", errors.New(err)
	}

	return fmt.Sprintf("%s___%s", prefix, strings.Replace(id.String(), "-", "", -1)), nil
}

func (k *kernel) transpile(filename string) (*string, error) {
	transpiler, err := newTranspiler(k)
	if err != nil {
		return nil, errors.New(err)
	}
	return transpiler.transpileFile(filename)
}

func (k *kernel) loadContent(filename string) ([]byte, error) {
	if k.kernelDebugging {
		fmt.Println(fmt.Sprintf("Kernel: Loading scriptfile: %s", filename))
	}
	if strings.HasSuffix(filename, ".gz") {
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Kernel: GZIP Decompressing scriptfile: %s", filename))
		}
		if b, err := ioutil.ReadFile(filename); err != nil {
			return nil, err
		} else {
			if reader, err := gzip.NewReader(bytes.NewReader(b)); err != nil {
				return nil, err
			} else {
				return ioutil.ReadAll(reader)
			}
		}
	}
	if strings.HasSuffix(filename, ".bz2") {
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Kernel: BZIP Decompressing scriptfile: %s", filename))
		}
		if b, err := ioutil.ReadFile(filename); err != nil {
			return nil, err
		} else {
			return ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(b)))
		}
	}
	return ioutil.ReadFile(filename)
}

func (k *kernel) loadScriptModule(id, name, filename, parentPath string, bundle *bundle) (Module, error) {
	if !filepath.IsAbs(filename) {
		filename = findScriptFile(filename, parentPath)
	}

	source, err := k.loadSource(filename)
	if err != nil {
		return nil, errors.New(err)
	}

	module := bundle.findModuleById(id)
	if module == nil {
		origin := newOrigin(filename)
		module, err = newModule(id, name, origin, bundle)
		if err != nil {
			return nil, errors.New(err)
		}
		bundle.addModule(module)
	}

	bundle.pushLoaderStack(id)

	// We expect a cleanly compiled module, that doesn't return anything
	val, err := prepareJavascript(filename, source, bundle.getSandbox())

	bundle.popLoaderStack()

	if err != nil {
		return nil, errors.New(err)
	}

	if val != goja.Undefined() && val != goja.Null() {
		return nil, errors.New(fmt.Sprintf("Modules are not supposed to return anything: %s", val.Export()))
	}

	return module, nil
}

func (k *kernel) kernelRegisterModule(module *module,
	dependencies []string, callback registerCallback, bundle *bundle) error {

	if k.kernelDebugging {
		fmt.Println(fmt.Sprintf("Loading %s into %s", module.name, bundle.Name()))
	}

	exportFunction := func(name string, value goja.Value) {
		module.getModuleExports().Set(name, value)
	}

	dependentModules := make([]Module, len(dependencies))
	for i, filename := range dependencies {
		moduleFile := findScriptFile(filename, module.origin.Path())

		if dependentModule := bundle.findModuleByModuleFile(moduleFile); dependentModule == nil {
			id, err := uuid.NewV4()
			if err != nil {
				return err
			}

			moduleId := id.String()
			m, err := k.loadScriptModule(moduleId, filename, filename, module.origin.Path(), bundle)
			if err != nil {
				panic(err)
			}
			dependentModules[i] = m

		} else {
			if k.kernelDebugging {
				fmt.Println(fmt.Sprintf("Reused already loaded module %s with id %s", filename, dependentModule.ID()))
			}
			dependentModules[i] = dependentModule
		}
	}

	context := module.Bundle().NewObject()
	context.Set("id", module.ID())

	initializer := callback(exportFunction, context)

	var setters []goja.Callable
	if err := module.export(initializer.Get("setters"), &setters); err != nil {
		panic(err)
	}

	execute := initializer.Get("execute")

	for i, setter := range setters {
		m := dependentModules[i]
		if _, err := setter(goja.Undefined(), m.getModuleExports()); err != nil {
			panic(err)
		}
	}

	var executable goja.Callable
	if err := module.export(execute, &executable); err != nil {
		panic(err)
	}

	// Register the actual classes
	if k.kernelDebugging {
		fmt.Println(fmt.Sprintf("Executing initializer of module: %s", module.Name()))
	}
	if _, err := executable(execute); err != nil {
		panic(err)
	}

	return nil
}

func (k *kernel) adaptBundleExports(origin Bundle, caller Bundle) (*goja.Object, error) {
	target := caller.NewObject()
	err := k.adaptBundleObject(origin.getBundleExports(), target, origin, caller)
	if err != nil {
		return nil, err
	}
	return target, nil
}

func (k *kernel) adaptBundleObject(source *goja.Object, target *goja.Object, origin Bundle, caller Bundle) error {
	return origin.getAdapter().adapt(source, target, origin, caller)
}
