package gomini

import (
	"github.com/dop251/goja"
	"path/filepath"
	"github.com/satori/go.uuid"
	"fmt"
	"github.com/go-errors/errors"
)

type mmExporter func(name string, value goja.Value)
type mmCallback func(export mmExporter, context *goja.Object) *goja.Object

type moduleManager struct {
	kernel  *kernel
	modules []Module
}

func newModuleManager(kernel *kernel) *moduleManager {
	return &moduleManager{
		kernel:  kernel,
		modules: make([]Module, 0),
	}
}

func (mm *moduleManager) loadPlainJavascript(file, path string, vm *goja.Runtime) (goja.Value, error) {
	filename := findScriptFile(file, path)
	if source, err := mm.kernel.loadSource(filename); err != nil {
		return nil, err
	} else {
		return prepareJavascript(filename, source, vm)
	}
}

func (mm *moduleManager) registerDefaults(module Module) error {
	console := module.getVm().NewObject()
	if err := console.Set("log", func(msg string) {
		fmt.Println(msg)
	}); err != nil {
		return err
	}
	module.getVm().Set("console", console)
	module.getVm().Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		return goja.Null()
	})
	if _, err := mm.loadPlainJavascript("js/kernel/promise.js", mm.baseDir(), module.getVm()); err != nil {
		return err
	}
	if _, err := mm.loadPlainJavascript("js/kernel/system.js", mm.baseDir(), module.getVm()); err != nil {
		return err
	}
	return nil
}

func (mm *moduleManager) registerSystemObject(module, parentModule Module) error {
	mm.pushModule(module)
	system := module.NewObject()
	if err := system.Set("register", mm.kernel.generateSystemRegister(module, parentModule)); err != nil {
		return errors.New(err)
	}
	module.Define("System", system)
	return nil
}

func (mm *moduleManager) defineKernelModule(module Module, moduleName, filename string, exporter func(exports *goja.Object)) {

	// Define final module name
	module.setName(moduleName)

	// Load the script definition file
	_, err := mm.loadScriptModule(module.ID(), module.Name(), filename, mm.kernel)
	if err != nil {
		panic(errors.New(err))
	}

	// Override exports
	exporter(module.getExports())

	// Freeze module
	module.FreezeObject(module.getExports())
}

func (mm *moduleManager) registerModule(name string, dependencies []string,
	callback mmCallback, currentModule, parentModule Module) goja.Value {

	if mm.kernel.kernelDebugging {
		parent := stringModuleOrigin(parentModule)
		file := stringModuleOrigin(currentModule)
		fmt.Println(fmt.Sprintf("Loading %s from parent %s", file, parent))
	}

	module := mm.findModuleById(currentModule.ID())
	module.setName(name)

	exportFunction := func(name string, value goja.Value) {
		module.getExports().Set(name, value)
	}

	dependentModules := make([]Module, len(dependencies))
	for i, filename := range dependencies {
		moduleFile := findScriptFile(filename, currentModule.Origin().Path())

		if dependentModule := mm.findModuleByModuleFile(moduleFile); dependentModule == nil {
			m, err := mm.loadScriptModule(uuid.NewV4().String(), "anonymous", filename, currentModule)
			if err != nil {
				panic(err)
			}
			dependentModules[i] = m

		} else {
			if mm.kernel.kernelDebugging {
				fmt.Println(fmt.Sprintf("Reused already loaded module %s with id %s", filename, dependentModule.ID()))
			}
			dependentModules[i] = dependentModule
		}
	}

	context := module.NewObject()
	context.Set("id", module.ID())

	initializer := callback(exportFunction, context)

	var setters []goja.Callable
	if err := module.Export(initializer.Get("setters"), &setters); err != nil {
		panic(err)
	}

	execute := initializer.Get("execute")

	for i, setter := range setters {
		m := dependentModules[i]
		adaptedExports, err := mm.adaptExports(m, module)
		if err != nil {
			panic(err)
		}
		if _, err := setter(goja.Undefined(), adaptedExports); err != nil {
			panic(err)
		}
	}

	var executable goja.Callable
	if err := module.Export(execute, &executable); err != nil {
		panic(err)
	}

	// Register the actual classes
	if mm.kernel.kernelDebugging {
		fmt.Println(fmt.Sprintf("Executing initializer of module: %s", module.Name()))
	}
	if _, err := executable(execute); err != nil {
		panic(err)
	}

	return goja.Undefined()
}

func (mm *moduleManager) adaptExports(origin Module, caller Module) (*goja.Object, error) {
	target := caller.NewObject()
	err := mm.adaptObject(origin.getExports(), target, origin, caller)
	if err != nil {
		return nil, err
	}
	return target, nil
}

func (mm *moduleManager) adaptObject(source *goja.Object, target *goja.Object, origin Module, caller Module) error {
	return origin.getAdapter().adapt(source, target, origin, caller)
}

func (mm *moduleManager) loadScriptModule(id, name, filename string, parentModule Module) (Module, error) {
	if !filepath.IsAbs(filename) {
		filename = findScriptFile(filename, parentModule.Origin().Path())
	}

	source, err := mm.kernel.loadSource(filename)
	if err != nil {
		return nil, errors.New(err)
	}

	module := mm.findModuleById(id)
	if module == nil {
		if s, err := newSandbox(mm.kernel, id, name, filename, false, parentModule); err != nil {
			return nil, errors.New(err)
		} else {
			module = s
		}
	}

	// We expect a cleanly compiled module, that doesn't return anything
	val, err := prepareJavascript(filename, []byte(source), module.getVm())
	if err != nil {
		return nil, errors.New(err)
	}

	if val != goja.Undefined() && val != goja.Null() {
		return nil, errors.New(fmt.Sprintf("Modules are not supposed to return anything: %s", val.Export()))
	}

	return module, nil
}

func (mm *moduleManager) pushModule(module Module) {
	mm.modules = append(mm.modules, module)
}

func (mm *moduleManager) findModuleById(id string) Module {
	for _, module := range mm.modules {
		if module.ID() == id {
			return module
		}
	}
	return nil
}

func (mm *moduleManager) findModuleByModuleFile(file string) Module {
	filename := filepath.Base(file)
	path := filepath.Dir(file)
	for _, module := range mm.modules {
		if module.Origin().Filename() == filename && module.Origin().Path() == path {
			return module
		}
	}
	return nil
}

func (mm *moduleManager) baseDir() string {
	return mm.kernel.baseDir
}
