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
	"github.com/spf13/afero"
	"github.com/relationsone/bacc"
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
	osfs            afero.Fs
	bundleManager   *bundleManager
	keyManager      bacc.KeyManager
	kernelDebugging bool
	resourceLoader  ResourceLoader
	scriptCache     map[string]*goja.Program
}

func NewScriptKernel(osfs, bundlefs afero.Fs, kernelDebugging bool) (*kernel, error) {
	kernel := &kernel{
		osfs:            osfs,
		kernelDebugging: kernelDebugging,
		resourceLoader:  newResourceLoader(),
		scriptCache:     make(map[string]*goja.Program),
	}

	kernel.bundleManager = newBundleManager(kernel)
	bundle, err := newBundle(kernel, "/", bundlefs, kernel_id, "kernel", []string{})
	if err != nil {
		return nil, err
	}

	kernel.bundle = bundle
	if err := kernel.bundle.init(kernel); err != nil {
		return nil, errors.New(err)
	}

	// Pre-transpile all typescript sourcefiles that are out of date
	if transpiler, err := newTranspiler(kernel); err != nil {
		return nil, errors.New(err)
	} else {
		if err := transpiler.transpileAll(kernel, "/"); err != nil {
			return nil, errors.New(err)
		}
	}

	return kernel, nil
}

func (k *kernel) Start() error {
	if err := k.bundleManager.start(); err != nil {
		return err
	}
	return nil
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
	filename := k.resolveScriptPath(k, kernelModule.ApiDefinitionFile())

	origin := newOrigin(filename)
	module, err := newModule(kernelModule.ID(), kernelModule.Name(), origin, k)
	if err != nil {
		return err
	}
	module.kernel = true
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

	_, err = k.loadScriptModule(id.String(), "entrypoint", filename, "/", k.bundle)
	if err != nil {
		return err
	}

	return nil
}

func (k *kernel) defineKernelModule(module Module, filename string, exporter func(exports *goja.Object)) {
	// Load the script definition file
	_, err := k.loadScriptModule(module.ID(), module.Name(), filename, "/", k.bundle)
	if err != nil {
		panic(errors.New(err))
	}

	// Override goExports
	exporter(module.getModuleExports())

	// Freeze module
	module.Bundle().FreezeObject(module.getModuleExports())
}

func (k *kernel) loadSource(bundle Bundle, filename string) (string, error) {
	if isTypeScript(filename) {
		// Is pre-transpiled?
		cacheFilename := filepath.Join(cacheVfsPath, tsCacheFilename(filename, bundle, k))
		if !fileExists(k.Filesystem(), cacheFilename) {
			if k.kernelDebugging {
				fmt.Println(fmt.Sprintf("Kernel: Loading scriptfile '%s:/%s' with live transpiler", bundle.Name(), filename))
			}

			source, err := k.transpile(bundle, filename)
			if err != nil {
				return "", err
			}

			// DEBUG
			if k.kernelDebugging && source != nil {
				fmt.Println(*source)
			}
			return *source, nil
		}

		// Override filename with the pre-transpiled, cached file
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Kernel: Loading scriptfile '%s:/%s' from pretranspiled cache: kernel:/%s", bundle.Name(), filename, cacheFilename))
		}

		if data, err := k.loadContent(bundle, k.Filesystem(), cacheFilename); err != nil {
			return "", err
		} else {
			return string(data), nil
		}
	}
	if data, err := k.loadContent(bundle, bundle.Filesystem(), filename); err != nil {
		return "", err
	} else {
		return string(data), nil
	}
}

func (k *kernel) transpile(bundle Bundle, filename string) (*string, error) {
	transpiler, err := newTranspiler(k)
	if err != nil {
		return nil, errors.New(err)
	}
	return transpiler.transpileFile(bundle, filename)
}

func (k *kernel) loadContent(bundle Bundle, filesystem afero.Fs, filename string) ([]byte, error) {
	if k.kernelDebugging {
		fmt.Println(fmt.Sprintf("Kernel: Loading content from scriptfile '%s:/%s'", bundle.Name(), filename))
	}
	b, err := k.resourceLoader.LoadResource(k, filesystem, filename)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(filename, ".gz") {
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Kernel: GZIP Decompressing scriptfile: %s:/%s", bundle.Name(), filename))
		}

		if reader, err := gzip.NewReader(bytes.NewReader(b)); err != nil {
			return nil, err
		} else {
			return ioutil.ReadAll(reader)
		}
	} else if strings.HasSuffix(filename, ".bz2") {
		if k.kernelDebugging {
			fmt.Println(fmt.Sprintf("Kernel: BZIP Decompressing scriptfile: %s:/%s", bundle.Name(), filename))
		}
		return ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(b)))
	}
	return b, nil
}

func (k *kernel) loadScriptModule(id, name, filename, parentPath string, bundle *bundle) (Module, error) {
	loadingBundle := bundle

	if filename[0] != '/' {
		return nil, errors.New("only absolute path is supported")
	}

	// TODO --------
	if !strings.HasPrefix(filename, "./") &&
		!strings.HasPrefix(filename, "../") &&
		!strings.HasPrefix(filename, "/") {

		filename = filepath.Join("/kernel/@types", filename)
		loadingBundle = k.bundle
	}

	if !filepath.IsAbs(filename) {
		filename = filepath.Join(parentPath, filename)
		filename = k.resolveScriptPath(bundle, filename)
	}
	// TODO --------

	prog, err := k.loadScriptSource(loadingBundle, filename, true)
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
	val, err := executeJavascript(prog, bundle)

	bundle.popLoaderStack()

	if err != nil {
		return nil, errors.New(err)
	}

	if val != goja.Undefined() && val != goja.Null() {
		return nil, errors.New(fmt.Sprintf("Modules are not supposed to return anything: %s", val.Export()))
	}

	return module, nil
}

func (k *kernel) kernelRegisterModule(module *module, dependencies []string, callback registerCallback, bundle *bundle) error {
	if k.kernelDebugging {
		fmt.Println(fmt.Sprintf("Kernel: Loading module %s (%s) into bundle %s (%s)", module.name, module.id, bundle.name, bundle.id))
	}

	exportFunction := func(name string, value goja.Value) {
		module.getModuleExports().Set(name, value)
	}

	dependentModules := make([]Module, len(dependencies))
	for i, filename := range dependencies {
		moduleFile := k.resolveScriptPath(bundle, filename)

		vfs, file, err := k.toVirtualKernelFile(moduleFile, bundle)
		if err != nil {
			return err
		}

		if vfs {
			fmt.Println("Kernel: Needs kernel intervention to get exported modules")
			fmt.Println("Kernel: ATTENTION //TODO// no proxy implementation between sandboxes yet!")
			if err == nil {
				dependentModules[i] = file.module
				continue
			}
		}

		if dependentModule := bundle.findModuleByModuleFile(moduleFile); dependentModule == nil {
			id, err := uuid.NewV4()
			if err != nil {
				return err
			}

			moduleId := id.String()
			m, err := k.loadScriptModule(moduleId, filename, moduleFile, module.origin.Path(), bundle)
			if err != nil {
				panic(err)
			}
			dependentModules[i] = m

		} else {
			if k.kernelDebugging {
				fmt.Println(fmt.Sprintf("Kernel: Reused already loaded module %s (%s:/%s) with id %s", filename, dependentModule.bundle.Name(), moduleFile, dependentModule.ID()))
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
		fmt.Println(fmt.Sprintf("Kernel: Executing initializer of module: %s", module.Name()))
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

func (k *kernel) loadScriptSource(bundle Bundle, filename string, allowCaching bool) (*goja.Program, error) {
	var prog *goja.Program
	if allowCaching {
		// TODO Fix to use kernel namespaced filename (to prevent apps to load known kernel files)
		fmt.Println(fmt.Sprintf("Kernel: Reusing preloaded bytecode for %s:/%s", bundle.Name(), filename))
		prog = k.scriptCache[filename]
	}

	if prog == nil {
		source, err := k.loadSource(bundle, filename)
		if err != nil {
			return nil, err
		}

		prog, err = compileJavascript(filename, source)
		if err != nil {
			return nil, err
		}

		if prog != nil && allowCaching {
			if t, err := goja.ExportProgram(prog, 1); err != nil {
				panic(err)
			} else {
				if _, err := goja.ReadProgram(bytes.NewReader(t), 1); err != nil {
					panic(err)
				}
			}
			k.scriptCache[filename] = prog
		}
	}

	if prog == nil {
		return nil, fmt.Errorf("could not load script file: %s:/%s", bundle.Name(), filename)
	}

	return prog, nil
}

func (k *kernel) toVirtualKernelFile(name string, bundle Bundle) (bool, *exportFile, error) {
	f, err := bundle.Filesystem().Open(name)
	if err != nil {
		return false, nil, err
	}
	switch ff := f.(type) {
	case *compositeFile:
		e, success := ff.file.(*exportFile)
		return success && !e.dir, e, nil
	}
	return false, nil, nil
}

func (k *kernel) toKernelPath(path string, bundle Bundle) string {
	if k.bundle == bundle {
		return path
	}

	basePath := bundle.getBasePath()
	return filepath.Join(basePath, path)
}

func (k *kernel) resolveScriptPath(bundle Bundle, filename string) string {
	filesystem := bundle.Filesystem()

	originalFilename := filename

	// Is non-relative and non-absolute? Non-relative paths are assumed to be an exported module
	if !strings.HasPrefix(filename, "./") &&
		!strings.HasPrefix(filename, "../") &&
		!strings.HasPrefix(filename, "/") {

		filename = filepath.Join("/kernel/@types", filename)
	}

	parent := "/"
	if bundle.peekLoaderStack() != "" {
		parentUuid := bundle.peekLoaderStack()
		parentModule := bundle.findModuleById(parentUuid)
		parent = parentModule.origin.Path()
	}

	// Clean path (removes ../ and ./)
	filename = filepath.Clean(filename)
	filename = filepath.Join(parent, filename)

	// See if we already have an extension
	if ext := filepath.Ext(filename); ext != "" {
		// If filename exists, we can stop here
		if fileExists(filesystem, filename) {
			return filename
		}
	}
	candidate := filename + ".ts"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filepath.Join(filename, "index.ts")
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".d.ts"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filepath.Join(filename, "index.d.ts")
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".js"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".js.gz"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".ts.gz"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".js.bz2"
	if fileExists(filesystem, candidate) {
		return candidate
	}
	candidate = filename + ".ts.bz2"
	if fileExists(filesystem, candidate) {
		return candidate
	}

	if !strings.HasPrefix(originalFilename, "./") &&
		!strings.HasPrefix(originalFilename, "../") &&
		!strings.HasPrefix(originalFilename, "/") {

		parent := k.peekLoaderStack()
		if parent == "" {
			parent = "/"
		}

		localImport := filepath.Join(parent, originalFilename+".d.ts")
		if fileExists(filesystem, localImport) {
			return localImport
		}
	}

	return filename
}

type resolvedScriptPath struct {
	path   string
	loader Bundle
}
