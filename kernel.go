package gomini

import (
	"github.com/dop251/goja"
	"reflect"
	"path/filepath"
	"fmt"
	"strings"
	"github.com/go-errors/errors"
	"io/ioutil"
	"compress/gzip"
	"compress/bzip2"
	"bytes"
)

const kernel_id = "76141a6c-0aec-4973-b04b-8fdd54753e03"

type kernel struct {
	vm                 *goja.Runtime
	mm                 *moduleManager
	baseDir            string
	transpilerCacheDir string
	kernelDebugging    bool
	propertyDefiner    goja.Callable
	constantDefiner    set_constant
	propertyDescriptor get_property
	adapter            *adapter
}

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
		vm:                 goja.New(),
		baseDir:            baseDir,
		transpilerCacheDir: transpilerCacheDir,
		kernelDebugging:    kernelDebugging,
	}

	kernel.mm = newModuleManager(kernel)

	kernel.constantDefiner = prepareDefineConstant(kernel.vm)
	kernel.propertyDefiner = prepareDefineProperty(kernel.vm)
	kernel.propertyDescriptor = preparePropertyDescriptor(kernel.vm)

	if err := kernel.mm.registerDefaults(kernel); err != nil {
		return nil, err
	}
	if err := kernel.mm.registerSystemObject(kernel, nil); err != nil {
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

func (kernel *kernel) LoadKernelModule(kernelModule KernelModuleDefinition) error {
	filename := findScriptFile(kernelModule.ApiDefinitionFile(), kernel.baseDir)

	module, err := newSandbox(kernel, kernelModule.ID(), kernelModule.Name(), filename, true, kernel)
	if err != nil {
		return errors.New(err)
	}
	module.interceptor = kernelModule.SecurityInterceptor()

	moduleBuilder := newModuleBuilder(module, kernel)
	binder := kernelModule.ExtensionBinder()
	binder(module, moduleBuilder)

	return nil
}

func (kernel *kernel) EntryPoint(filename string) error {
	filename = findScriptFile(filename, kernel.baseDir)

	source, err := kernel.loadSource(filename)
	if err != nil {
		return errors.New(err)
	}

	prog, err := compileJavascript(filename, source)
	if err != nil {
		return errors.New(err)
	}

	_, err = kernel.vm.RunProgram(prog)
	if err != nil {
		return errors.New(err)
	}

	return nil
}

func (kernel *kernel) ID() string {
	return kernel_id
}

func (kernel *kernel) Name() string {
	return "kernel"
}

func (kernel *kernel) Origin() Origin {
	return &moduleOrigin{
		filename: "kernel",
		path:     kernel.baseDir,
	}
}

func (kernel *kernel) Exports() map[string]interface{} {
	return make(map[string]interface{}, 0)
}

func (kernel *kernel) Privileged() bool {
	return true
}

func (kernel *kernel) SecurityInterceptor() SecurityInterceptor {
	return kernel.securityInterceptor
}

func (kernel *kernel) NewObject() *goja.Object {
	return kernel.vm.NewObject()
}

func (kernel *kernel) Define(property string, value interface{}) {
	kernel.vm.Set(property, value)
}

func (kernel *kernel) DefineProperty(object *goja.Object, property string, value interface{}, getter Getter, setter Setter) {
	//kernel.propertyDefiner(object, property, value, getter, setter)
	callPropertyDefiner(kernel.propertyDefiner, kernel.vm, object, property, value, getter, setter)
}

func (kernel *kernel) DefineConstant(object *goja.Object, constant string, value interface{}) {
	kernel.constantDefiner(object, constant, value)
}

func (kernel *kernel) PropertyDescriptor(object *goja.Object, property string) (interface{}, bool, Getter, Setter) {
	descriptor := kernel.propertyDescriptor(object, property)
	return propertyDescriptor(kernel.vm, descriptor.ToObject(kernel.vm))
}

func (kernel *kernel) Export(value goja.Value, target interface{}) error {
	return kernel.vm.ExportTo(value, target)
}

func (kernel *kernel) ToValue(value interface{}) goja.Value {
	return kernel.vm.ToValue(value)
}

func (kernel *kernel) FreezeObject(object *goja.Object) {
	_freezeObject(kernel.ToValue(object), kernel.vm)
}

func (kernel *kernel) getExports() *goja.Object {
	return goja.Undefined().ToObject(kernel.vm)
}

func (kernel *kernel) securityInterceptor(caller Module, property string) bool {
	return caller.Privileged()
}

func (kernel *kernel) setName(name string) {
}

func (kernel *kernel) getVm() *goja.Runtime {
	return kernel.vm
}

func (kernel *kernel) getAdapter() *adapter {
	return kernel.adapter
}

func (kernel *kernel) generateSystemRegister(module, parentModule Module) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		argIndex := 0

		name := stringModuleOrigin(module)

		argument := call.Argument(argIndex)
		switch argument.ExportType().Kind() {
		case reflect.String:
			name = argument.String()
			argIndex++
		}

		argument = call.Argument(argIndex)
		if !isArray(argument) {
			panic("Neither string (name) or array (dependencies) was passed as the first parameter")
		}
		argIndex++

		deps := argument.Export().([]interface{})
		dependencies := make([]string, len(deps))
		for i := 0; i < len(deps); i++ {
			dependencies[i] = deps[i].(string)
		}

		var callback mmCallback
		module.Export(call.Argument(argIndex), &callback)

		return kernel.mm.registerModule(name, dependencies, callback, module, parentModule)
	}
}

func (kernel *kernel) loadSource(filename string) ([]byte, error) {
	if isTypeScript(filename) {
		// Is pre-transpiled?
		cacheFilename := filepath.Join(kernel.transpilerCacheDir, hash(filename))
		if !fileExists(cacheFilename) {
			if kernel.kernelDebugging {
				fmt.Println(fmt.Sprintf("Loading scriptfile '%s' with live transpiler", filename))
			}

			source, err := kernel.transpile(filename)
			if err != nil {
				return nil, err
			}

			// DEBUG
			if kernel.kernelDebugging {
				fmt.Println(*source)
			}
			return []byte(*source), nil
		}

		// Override filename with the pre-transpiled, cached file
		if kernel.kernelDebugging {
			fmt.Println(fmt.Sprintf("Loading scriptfile '%s' from pretranspiled cache: %s", filename, cacheFilename))
		}

		filename = cacheFilename
	}
	if data, err := kernel.loadContent(filename); err != nil {
		return nil, err
	} else {
		return data, nil
	}
}

func (kernel *kernel) transpile(filename string) (*string, error) {
	transpiler, err := newTranspiler(kernel)
	if err != nil {
		return nil, errors.New(err)
	}
	return transpiler.transpileFile(filename)
}

func (kernel *kernel) loadContent(filename string) ([]byte, error) {
	if kernel.kernelDebugging {
		fmt.Println(fmt.Sprintf("Loading scriptfile: %s", filename))
	}
	if strings.HasSuffix(filename, ".gz") {
		if kernel.kernelDebugging {
			fmt.Println(fmt.Sprintf("GZIP Decompressing scriptfile: %s", filename))
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
		if kernel.kernelDebugging {
			fmt.Println(fmt.Sprintf("BZIP	 Decompressing scriptfile: %s", filename))
		}
		if b, err := ioutil.ReadFile(filename); err != nil {
			return nil, err
		} else {
			return ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(b)))
		}
	}
	return ioutil.ReadFile(filename)
}
