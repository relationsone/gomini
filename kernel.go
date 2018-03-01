package gomini

import (
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
	"github.com/apex/log"
)

const kernel_id = "76141a6c-0aec-4973-b04b-8fdd54753e03"

type registerCallback func(export func(name string, value Value), context Object) Object

/*
 The kernel is a special bundle type, which is the root bundle to be initialized and has
 all privileges (PRIVILEGE_KERNEL) and can leave bundle boundaries.
 */
type kernel struct {
	*bundle
	osfs           afero.Fs
	bundleManager  *bundleManager
	keyManager     bacc.KeyManager
	resourceLoader ResourceLoader
	sandboxFactory SandboxFactory
	scriptCache    map[string]Script
}

func NewScriptKernel(osfs, bundlefs afero.Fs, sandboxFactory SandboxFactory, apiBinders []ApiProviderBinder) (*kernel, error) {
	//log.Infof("Gomini is starting...")
	for _, line := range strings.Split(bannerLarge, "\n") {
		log.Info(line)
	}

	kernel := &kernel{
		osfs:           osfs,
		resourceLoader: newResourceLoader(),
		sandboxFactory: sandboxFactory,
		scriptCache:    make(map[string]Script),
	}

	apiBinders = append(apiBinders, consoleApi(), timeoutApi())

	kernel.bundleManager = newBundleManager(kernel, apiBinders)
	bundle, err := newBundle(kernel, "/", bundlefs, kernel_id, "kernel", []string{})
	if err != nil {
		return nil, err
	}

	kernel.bundle = bundle
	if err := kernel.bundle.init(kernel); err != nil {
		return nil, errors.New(err)
	}

	_, err = kernel.sandbox.NewDebugger()
	if err != nil {
		return nil, err
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

		moduleName := strings.ToUpper(strings.Split(property, ".")[0])
		privilege := fmt.Sprintf("PRIVILEGE_%s", moduleName)
		privileges := caller.Privileges()
		for _, p := range privileges {
			if p == privilege {
				return true
			}
		}

		//TODO: add real checks here
		return false
	}
}

func (k *kernel) LoadKernelModule(kernelModule KernelModule) error {
	scriptPath := k.resolveScriptPath(k, kernelModule.ApiDefinitionFile())

	origin := newOrigin(scriptPath.path)
	module, err := newModule(kernelModule.ID(), kernelModule.Name(), origin, k)
	if err != nil {
		return err
	}
	module.kernel = true
	k.addModule(module)

	k.defineKernelModule(module, module.Origin().FullPath(), func(exports Object) {
		binder := kernelModule.KernelModuleBinder()
		objectCreator := k.sandbox.NewObjectCreator(kernelModule.Name())
		binder(k, objectCreator)
		objectCreator.BuildInto("", exports)
	})

	return nil
}

func (k *kernel) EntryPoint(filename string) error {
	k.setBundleStatus(BundleStatusStarting)
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	_, err = k.loadScriptModule(id.String(), "entrypoint", "/", &resolvedScriptPath{filename, k.bundle}, k.bundle)
	if err != nil {
		return err
	}

	k.setBundleStatus(BundleStatusStarted)
	return nil
}

func (k *kernel) defineKernelModule(module Module, filename string, exporter func(exports Object)) {
	// Load the script definition file
	_, err := k.loadScriptModule(module.ID(), module.Name(), "/", &resolvedScriptPath{filename, k.bundle}, k.bundle)
	if err != nil {
		panic(errors.New(err))
	}

	// Override goExports
	exporter(module.getModuleExports())

	// Freeze module
	module.Bundle().FreezeObject(module.getModuleExports())
}

func (k *kernel) loadScriptModule(id, name, parentPath string, scriptPath *resolvedScriptPath, bundle *bundle) (Module, error) {
	//loadingBundle := bundle

	filename := scriptPath.path
	if filename[0] != '/' {
		return nil, errors.New("only absolute path is supported")
	}

	prog, err := k.loadScriptSource(scriptPath, true)
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
	val, err := bundle.Sandbox().Execute(prog)

	bundle.popLoaderStack()

	if err != nil {
		return nil, errors.New(err)
	}

	if val != bundle.Undefined() && val != bundle.Null() {
		return nil, errors.New(fmt.Sprintf("Modules are not supposed to return anything: %s", val.Export()))
	}

	return module, nil
}

func (k *kernel) loadContent(bundle Bundle, filesystem afero.Fs, filename string) ([]byte, error) {
	log.Debugf("Kernel: Loading content from scriptfile '%s:/%s'", bundle.Name(), filename)

	b, err := k.resourceLoader.LoadResource(k, filesystem, filename)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(filename, ".gz") {
		log.Debugf("Kernel: GZIP Decompressing scriptfile: %s:/%s", bundle.Name(), filename)

		if reader, err := gzip.NewReader(bytes.NewReader(b)); err != nil {
			return nil, err
		} else {
			defer reader.Close()
			return ioutil.ReadAll(reader)
		}
	} else if strings.HasSuffix(filename, ".bz2") {
		log.Debugf("Kernel: BZIP Decompressing scriptfile: %s:/%s", bundle.Name(), filename)
		return ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(b)))
	}
	return b, nil
}

func (k *kernel) registerModule(module *module, dependencies []string, callback registerCallback, bundle *bundle) error {
	log.Debugf("Kernel: Loading module %s (%s) into bundle %s (%s)", module.Name(), module.ID(), bundle.Name(), bundle.ID())

	exportFunction := func(name string, value Value) {
		module.getModuleExports().DefineConstant(name, value)
	}

	if len(dependencies) > 0 {
		log.Debugf("Kernel: Bundle %s has injection request: [%s]", bundle.Name(), strings.Join(dependencies, ", "))
	}

	dependentModules := make([]Module, len(dependencies))
	for i, dependency := range dependencies {
		dependentModule, err := k.__resolveDependencyModule(dependency, bundle, module)
		if err != nil {
			return err
		}

		dependentModules[i] = dependentModule
	}

	context := module.Bundle().NewObject()
	context.DefineConstant("id", module.ID())

	initializer := callback(exportFunction, context)

	var setters []Callable
	if err := module.export(initializer.Get("setters"), &setters); err != nil {
		panic(err)
	}

	execute := initializer.Get("execute")

	for i, setter := range setters {
		m := dependentModules[i]

		exports := m.getModuleExports()
		if m.Bundle().ID() != bundle.ID() {
			log.Debugf("Kernel: Create security proxy from '%s:/%s' to '%s:/%s'",
				bundle.Name(), module.Origin().FullPath(), m.Bundle().Name(), m.Origin().FullPath())

			moduleProxy, err := m.Bundle().Sandbox().NewModuleProxy(m.getModuleExports(), m.Name(), bundle)
			if err != nil {
				panic(err)
			}
			exports = moduleProxy
		}

		if _, err := setter(k.Undefined(), exports); err != nil {
			panic(err)
		}
	}

	var executable Callable
	if err := module.export(execute, &executable); err != nil {
		panic(err)
	}

	// Register the actual classes
	log.Debugf("Kernel: Executing initializer of module: %s", module.Name())

	if _, err := executable(execute); err != nil {
		panic(err)
	}

	return nil
}

func (k *kernel) loadScriptSource(scriptPath *resolvedScriptPath, allowCaching bool) (Script, error) {
	cacheFilename := tsCacheFilename(scriptPath.path, scriptPath.loader, k)

	log.Infof("Kernel: Loading script '%s:/%s'", scriptPath.loader.Name(), scriptPath.path)

	var prog Script
	if allowCaching {
		prog = k.scriptCache[cacheFilename]
		if prog != nil {
			log.Debugf("Kernel: Reusing preloaded bytecode for '%s:/%s'", scriptPath.loader.Name(), scriptPath.path)
		}
	}

	loaderName := fmt.Sprintf("%s:/%s", scriptPath.loader.Name(), scriptPath.path)

	if prog == nil {
		source, err := k.__loadSource(scriptPath.loader, scriptPath.path)
		if err != nil {
			return nil, err
		}

		var cacheable bool
		prog, cacheable, err = k.sandbox.Compile(loaderName, source)
		if err != nil {
			return nil, err
		}

		if prog != nil && cacheable && allowCaching {
			// TODO export bytecode
			/*if t, err := goja.ExportProgram(prog, 1); err != nil {
				panic(err)
			} else {
				if _, err := goja.ReadProgram(bytes.NewReader(t), 1); err != nil {
					panic(err)
				}
			}*/
			k.scriptCache[cacheFilename] = prog
		}
	}

	if prog == nil {
		return nil, fmt.Errorf("could not load script file: %s:/%s", scriptPath.loader, scriptPath.path)
	}

	return prog, nil
}

func (k *kernel) toKernelPath(path string, bundle Bundle) string {
	if k.bundle == bundle {
		return path
	}

	basePath := bundle.getBasePath()
	return filepath.Join(basePath, path)
}

func (k *kernel) resolveScriptPath(bundle Bundle, filename string) *resolvedScriptPath {
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

	if isJavaScript(filename) && !bundle.Privileged() {
		return nil
	}

	// See if we already have an extension
	if ext := filepath.Ext(filename); ext != "" {
		// If filename exists, we can stop here
		if fileExists(filesystem, filename) {
			return &resolvedScriptPath{filename, bundle}
		}
	}
	candidate := filename + ".ts"
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filepath.Join(filename, "index.ts")
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filename + ".d.ts"
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filepath.Join(filename, "index.d.ts")
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filename + ".ts.gz"
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filename + ".ts.bz2"
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filepath.Join(filename, "index.d.ts.gz")
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}
	candidate = filepath.Join(filename, "index.d.ts.bz2")
	if fileExists(filesystem, candidate) {
		return &resolvedScriptPath{candidate, bundle}
	}

	// Only privileged bundles are allowed to load plain JavaScript code after this point
	if bundle.Privileged() {
		candidate = filename + ".js"
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
		candidate = filename + ".js.gz"
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
		candidate = filename + ".js.bz2"
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
		candidate = filepath.Join(filename, "index.js")
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
		candidate = filepath.Join(filename, "index.js.gz")
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
		candidate = filepath.Join(filename, "index.js.bz2")
		if fileExists(filesystem, candidate) {
			return &resolvedScriptPath{candidate, bundle}
		}
	}

	// Try to resolve local definition files.
	// Those are resolved right before giving up to prevent to override kernel exports
	if !strings.HasPrefix(originalFilename, "./") &&
		!strings.HasPrefix(originalFilename, "../") &&
		!strings.HasPrefix(originalFilename, "/") {

		parent := k.peekLoaderStack()
		if parent == "" {
			parent = "/"
		}

		localImport := filepath.Join(parent, originalFilename+".d.ts")
		if fileExists(filesystem, localImport) {
			return &resolvedScriptPath{localImport, bundle}
		}
	}

	return &resolvedScriptPath{filename, bundle}
}

type resolvedScriptPath struct {
	path   string
	loader Bundle
}
