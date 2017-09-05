package gomini

import (
	"github.com/dop251/goja"
	"reflect"
	"os"
	"strings"
	"fmt"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"github.com/dop251/goja/parser"
	"github.com/go-errors/errors"
)

type set_property func(object *goja.Object, propertyName string, value interface{}, getter Getter, setter Setter)
type set_constant func(object *goja.Object, propertyName string, value interface{})
type get_property func(object *goja.Object, propertyName string) goja.Value

func stringModuleOrigin(module Module) string {
	if module == nil {
		return "no origin available"
	}

	path := module.Origin().Path()
	filename := module.Origin().Filename()

	return filepath.Join(path, filename)
}

func isFunction(value goja.Value) bool {
	return isDefined(value) && value.ExportType().Kind() == reflect.Func
}

func isString(value goja.Value) bool {
	return isDefined(value) && value.ExportType().Kind() == reflect.String
}

func isArray(value goja.Value) bool {
	kind := value.ExportType().Kind()
	return isDefined(value) && kind == reflect.Slice || kind == reflect.Array
}

func isDefined(value goja.Value) bool {
	return value != goja.Null() && value != goja.Undefined()
}

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func isTypeScript(filename string) bool {
	return strings.HasSuffix(filename, ".ts") ||
		strings.HasSuffix(filename, ".d.ts") ||
		strings.HasSuffix(filename, ".ts.gz") ||
		strings.HasSuffix(filename, ".d.ts.gz") ||
		strings.HasSuffix(filename, ".ts.bz2") ||
		strings.HasSuffix(filename, ".d.ts.bz2")
}

func hash(value string) string {
	hasher := sha256.New()
	hasher.Write([]byte(value))
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum)
}

func findScriptFile(filename string, baseDir string) string {
	if !strings.HasPrefix(filename, "zodiac/") {
		// Make it a full path
		filename = filepath.Join(baseDir, filename)
	} else {
		// Make it a full path
		filename = filepath.Join(baseDir, "..", "@types", filename)
	}

	// Clean path (removes ../ and ./)
	filename = filepath.Clean(filename)

	// See if we already have an extension
	if ext := filepath.Ext(filename); ext != "" {
		// If filename exists, we can stop here
		if fileExists(filename) {
			return filename
		}
	}
	candidate := filename + ".ts"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filepath.Join(filename, "index.ts")
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".d.ts"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filepath.Join(filename, "index.d.ts")
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".js"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".js.gz"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".ts.gz"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".js.bz2"
	if fileExists(candidate) {
		return candidate
	}
	candidate = filename + ".ts.bz2"
	if fileExists(candidate) {
		return candidate
	}
	return filename
}

func sandboxSecurityCheck(property string, origin Module, caller Module) {
	interceptor := origin.SecurityInterceptor()
	if !caller.Privileged() && interceptor != nil {
		if !interceptor(caller, property) {
			msg := fmt.Sprintf("Illegal access violation: %s cannot access %s::%s",
				caller.Origin().Filename(), origin.Origin().Filename(), property)
			panic(errors.New(msg))
		}
	}
	fmt.Println(fmt.Sprintf("SecurityInterceptor check success: %s", property))
}

func prepareJavascript(filename string, source []byte, runtime *goja.Runtime) (goja.Value, error) {
	if prog, err := compileJavascript(filename, source); err != nil {
		return nil, err

	} else {
		if val, err := runtime.RunProgram(prog); err != nil {
			return nil, err
		} else {
			return val, nil
		}
	}
}

func compileJavascript(filename string, source []byte) (*goja.Program, error) {
	if !strings.HasPrefix(filename, "system::") && !filepath.IsAbs(filename) {
		return nil, fmt.Errorf("Provided path is not absolute: %s", filename)
	}
	ast, err := parser.ParseFile(nil, filename, source, 0)
	if err != nil {
		return nil, err
	}

	return goja.CompileAST(ast, true)
}

func propertyDescriptor(vm *goja.Runtime, descriptor *goja.Object) (interface{}, bool, Getter, Setter) {
	value := descriptor.Get("value").Export()
	writeable := descriptor.Get("writable").ToBoolean()
	get := descriptor.Get("get")
	set := descriptor.Get("set")

	var getter Getter
	var setter Setter
	if get != nil && get != goja.Null() && get != goja.Undefined() {
		vm.ExportTo(get, &getter)
	}
	if set != nil && set != goja.Null() && set != goja.Undefined() {
		vm.ExportTo(set, &setter)
	}

	return value, writeable, getter, setter
}

func callPropertyDefiner(callable goja.Callable, vm *goja.Runtime, object *goja.Object, property string,
	value interface{}, getter Getter, setter Setter) {

	arguments := make([]goja.Value, 5)
	arguments[0] = vm.ToValue(object)
	arguments[1] = vm.ToValue(property)
	arguments[2] = goja.Null()
	if value != nil {
		arguments[2] = vm.ToValue(value)
	}
	arguments[3] = goja.Null()
	if getter != nil {
		arguments[3] = vm.ToValue(getter)
	}
	arguments[4] = goja.Null()
	if setter != nil {
		arguments[4] = vm.ToValue(setter)
	}
	_, err := callable(vm.ToValue(callable), arguments...)
	if err != nil {
		panic(err)
	}
}

func DeepFreezeObject(value goja.Value, vm *goja.Runtime) goja.Value {
	var deepFreeze goja.Callable
	function := vm.Get("deepFreeze")
	vm.ExportTo(function, &deepFreeze)

	if val, err := deepFreeze(function, value); err != nil {
		panic(err)
	} else {
		return val
	}
}

func _freezeObject(value goja.Value, vm *goja.Runtime) goja.Value {
	object := vm.Get("Object").ToObject(vm)

	var freeze goja.Callable
	object.Get("freeze")
	vm.ExportTo(object, &freeze)

	if val, err := freeze(object, value); err != nil {
		panic(err)
	} else {
		return val
	}
}

func prepareDefineProperty(vm *goja.Runtime) goja.Callable {
	source := `
	(function() {
		return function(parent, set_property, value, getter, setter) {
			var configuration = {
				//writable: (value && setter != null),
				enumerable: true,
				configurable: false,
			}

			if (value) {
				configuration.value = value;
			}

			if (getter) {
				configuration.get = function() {
					return getter();
				};
			}

			if (setter) {
				configuration.set = function(newValue) {
					setter(newValue);
				};
			}

			Object.defineProperty(parent, set_property, configuration);
		};
	})()`

	prog, err := compileJavascript("system::DefineProperty", []byte(source))
	if err != nil {
		panic(err)
	}

	if value, err := vm.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var property goja.Callable
		vm.ExportTo(value, &property)
		return property
	}
}

func prepareDefineConstant(vm *goja.Runtime) set_constant {
	source := `
	(function() {
		return function(parent, set_property, value) {
			Object.defineProperty(parent, set_property, {
				writable: false,
				enumerable: true,
				configurable: false,
				value: value
			});
		}
	})()
	`

	prog, err := compileJavascript("system::DefineConstant", []byte(source))
	if err != nil {
		panic(err)
	}

	if value, err := vm.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var constant set_constant
		vm.ExportTo(value, &constant)
		return constant
	}
}

func preparePropertyDescriptor(vm *goja.Runtime) get_property {
	source := `
	(function() {
		return function(object, set_property) {
			return Object.getOwnPropertyDescriptor(object, set_property)
		}
	})()
	`

	prog, err := compileJavascript("system::PropertyDescriptor", []byte(source))
	if err != nil {
		panic(err)
	}

	if value, err := vm.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var property get_property
		vm.ExportTo(value, &property)
		return property
	}
}
