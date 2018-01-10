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

func NewScriptKernel(baseDir, transpilerCacheDir string, kernelDebugging bool) (*kernel, error) {
	if !filepath.IsAbs(baseDir) {
		absBaseDir, err := filepath.Abs(baseDir)
		if err != nil {
			panic(err)
		} else {
			baseDir = absBaseDir
		}
	}

	if !strings.HasSuffix(baseDir, "/") {
		baseDir = baseDir + "/"
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
		sandbox:            goja.New(),
		baseDir:            baseDir,
		transpilerCacheDir: transpilerCacheDir,
		kernelDebugging:    kernelDebugging,
	}

	kernel.bundleManager = newBundleManager(kernel, baseDir)

	kernel.constantDefiner = prepareDefineConstant(kernel.sandbox)
	kernel.propertyDefiner = prepareDefineProperty(kernel.sandbox)
	kernel.propertyDescriptor = preparePropertyDescriptor(kernel.sandbox)

	if err := kernel.bundleManager.registerDefaults(kernel); err != nil {
		return nil, err
	}

	adapter, err := newAdapter(kernel, kernel)
	if err != nil {
		return nil, errors.New(err)
	}
	kernel.adapter = adapter

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

/**
 * The kernel is a special bundle type, which is the root bundle to be initialized and has
 * all privileges (PRIVILEGE_KERNEL) and can leave bundle boundaries.
 */
type kernel struct {
	sandbox            *goja.Runtime
	bundleManager      *bundleManager
	baseDir            string
	transpilerCacheDir string
	kernelDebugging    bool
	propertyDefiner    goja.Callable
	constantDefiner    set_constant
	propertyDescriptor get_property
	adapter            *adapter
	exports            *exportAdapter
	modules            []*module
}

func (k *kernel) findModuleByModuleFile(file string) *module {
	filename := filepath.Base(file)
	path := filepath.Dir(file)
	for _, module := range k.modules {
		if module.Origin().Filename() == filename && module.Origin().Path() == path {
			return module
		}
	}
	return nil
}

func (k *kernel) findModuleById(id string) *module {
	for _, module := range k.modules {
		if module.ID() == id {
			return module
		}
	}
	return nil
}

func (k *kernel) Path() string {
	return k.baseDir
}

func (k *kernel) BundleExports() ExportAdapter {
	return k.exports
}

func (k *kernel) getBundleExports() *goja.Object {
	return k.exports.jsExports
}

func (k *kernel) getSandbox() *goja.Runtime {
	return k.sandbox
}

func (k *kernel) LoadKernelModule(kernelModule KernelModuleDefinition) error {
	filename := findScriptFile(kernelModule.ApiDefinitionFile(), k.baseDir)

	origin := newOrigin(filename)
	module, err := newModule(kernelModule.ID(), kernelModule.Name(), origin, k)
	if err != nil {
		return err
	}

	moduleBuilder := newModuleBuilder(module, k)
	binder := kernelModule.ExtensionBinder()
	binder(k, moduleBuilder)

	return nil
}

func (k *kernel) EntryPoint(filename string) error {
	filename = findScriptFile(filename, k.baseDir)

	source, err := k.loadSource(filename)
	if err != nil {
		return errors.New(err)
	}

	id, err := uuid.NewV4()
	if err != nil {
		return err
	}

	origin := newOrigin(filename)
	module, err := newModule(id.String(), "entrypoint", origin, k)
	if err != nil {
		return err
	}

	prog, err := compileJavascript(filename, source)
	if err != nil {
		return errors.New(err)
	}

	/*val, err := k.sandbox.RunProgram(prog)
	if err != nil {
		return errors.New(err)
	}

	if val.ExportType().Kind() != reflect.Func {
		return errors.New(fmt.Sprintf("Modules are supposed to return a function: %s", val.Export()))
	}

	var call goja.Callable
	k.getSandbox().ExportTo(val, &call)

	retval, err := call(val, module.system)
	if err != nil {
		return errors.New(err)
	}

	if retval != goja.Undefined() && retval != goja.Null() {
		return errors.New(
			fmt.Sprintf("Modules initializers aren't supposed to return anything: %s", val.Export()))
	}*/

	return executeWithSystem(module, prog)
}

func (k *kernel) ID() string {
	return kernel_id
}

func (k *kernel) Name() string {
	return "kernel"
}

func (k *kernel) Origin() Origin {
	return &moduleOrigin{
		filename: "kernel",
		path:     k.baseDir,
	}
}

func (k *kernel) Exports() map[string]interface{} {
	return make(map[string]interface{}, 0)
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

func (k *kernel) NewObject() *goja.Object {
	return k.sandbox.NewObject()
}

func (k *kernel) Define(property string, value interface{}) {
	k.sandbox.Set(property, value)
}

func (k *kernel) DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter) {
	callPropertyDefiner(k.propertyDefiner, k.sandbox, object, property, value, getter, setter)
}

func (k *kernel) DefineConstant(object *goja.Object, constant string, value interface{}) {
	k.constantDefiner(object, constant, value)
}

func (k *kernel) PropertyDescriptor(object *goja.Object, property string) (interface{}, bool, Getter, Setter) {
	descriptor := k.propertyDescriptor(object, property)
	return propertyDescriptor(k.sandbox, descriptor.ToObject(k.sandbox))
}

func (k *kernel) Export(value goja.Value, target interface{}) error {
	return k.sandbox.ExportTo(value, target)
}

func (k *kernel) ToValue(value interface{}) goja.Value {
	return k.sandbox.ToValue(value)
}

func (k *kernel) FreezeObject(object *goja.Object) {
	_freezeObject(k.ToValue(object), k.sandbox)
}

func (k *kernel) getExports() *goja.Object {
	return goja.Undefined().ToObject(k.sandbox)
}

func (k *kernel) securityInterceptor(caller Module, property string) bool {
	return caller.Bundle().Privileged()
}

func (k *kernel) getVm() *goja.Runtime {
	return k.sandbox
}

func (k *kernel) getAdapter() *adapter {
	return k.adapter
}

func (k *kernel) registerModule(name string, dependencies []string, callback registerCallback, module Module) error {
	return k.kernelRegisterModule(name, dependencies, callback, module, k)
}

func (k *kernel) defineKernelModule(module Module, filename string, exporter func(exports *goja.Object)) {
	// Load the script definition file
	_, err := k.loadScriptModule(module.ID(), module.Name(), filename, k.baseDir, module.Bundle())
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

func (k *kernel) loadScriptModule(id, name, filename, parentPath string, bundle Bundle) (Module, error) {
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
	}

	prog, err := compileJavascript(filename, source)
	if err != nil {
		return nil, err
	}

	if err := executeWithSystem(module, prog); err != nil {
		return nil, err
	}

	return module, nil
	/*val, err := prepareJavascript(filename, source, bundle.getSandbox())
	if err != nil {
		return nil, errors.New(err)
	}

	bundle.getSandbox().R

	if val.ExportType().Kind() != reflect.Func {
		return nil, errors.New(fmt.Sprintf("Modules are supposed to return a function: %s", val.Export()))
	}

	var call goja.Callable
	bundle.getSandbox().ExportTo(val, &call)

	retval, err := call(val, module.system)
	if err != nil {
		return nil, errors.New(err)
	}

	if retval != goja.Undefined() && retval != goja.Null() {
		return nil, errors.New(
			fmt.Sprintf("Modules initializers aren't supposed to return anything: %s", val.Export()))
	}

	return module, nil*/
}

func (k *kernel) kernelRegisterModule(name string, dependencies []string,
	callback registerCallback, module Module, bundle Bundle) error {

	if k.kernelDebugging {
		file := stringModuleOrigin(module)
		fmt.Println(fmt.Sprintf("Loading %s from %s", name, file))
	}

	exportFunction := func(name string, value goja.Value) {
		module.getModuleExports().Set(name, value)
	}

	dependentModules := make([]Module, len(dependencies))
	for i, filename := range dependencies {
		moduleFile := findScriptFile(filename, module.Origin().Path())

		if dependentModule := bundle.findModuleByModuleFile(moduleFile); dependentModule == nil {
			id, err := uuid.NewV4()
			if err != nil {
				return err
			}

			moduleId := id.String()
			m, err := k.loadScriptModule(moduleId, filename, filename, module.Origin().Path(), bundle)
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
	if err := module.Export(initializer.Get("setters"), &setters); err != nil {
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
	if err := module.Export(execute, &executable); err != nil {
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
