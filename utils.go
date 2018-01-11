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
)

type set_property func(object *goja.Object, propertyName string, value interface{}, getter Getter, setter Setter)
type set_constant func(object *goja.Object, propertyName string, value interface{})
type get_property func(object *goja.Object, propertyName string) goja.Value

func stringOrigin(origin Origin) string {
	if origin == nil {
		return "no origin available"
	}

	path := origin.Path()
	filename := origin.Filename()

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

func loadPlainJavascript(kernel *kernel, file, path string, sandbox *goja.Runtime) (goja.Value, error) {
	filename := findScriptFile(file, path)
	if source, err := kernel.loadSource(filename); err != nil {
		return nil, err
	} else {
		return prepareJavascript(filename, source, sandbox)
	}
}

func findScriptFile(filename string, baseDir string) string {
	if !strings.HasPrefix(filename, "kernel/") {
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

func prepareJavascript(filename string, source string, runtime *goja.Runtime) (goja.Value, error) {
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

func compileJavascript(filename string, source string) (*goja.Program, error) {
	if !strings.HasPrefix(filename, "system::") && !filepath.IsAbs(filename) {
		return nil, fmt.Errorf("provided path is not absolute: %s", filename)
	}
	ast, err := parser.ParseFile(nil, filename, source, 0)
	if err != nil {
		return nil, err
	}

	return goja.CompileAST(ast, true)
}

func propertyDescriptor(runtime *goja.Runtime, descriptor *goja.Object) (interface{}, bool, Getter, Setter) {
	value := descriptor.Get("value").Export()
	writeable := descriptor.Get("writable").ToBoolean()
	get := descriptor.Get("get")
	set := descriptor.Get("set")

	var getter Getter
	var setter Setter
	if get != nil && get != goja.Null() && get != goja.Undefined() {
		runtime.ExportTo(get, &getter)
	}
	if set != nil && set != goja.Null() && set != goja.Undefined() {
		runtime.ExportTo(set, &setter)
	}

	return value, writeable, getter, setter
}

func callPropertyDefiner(callable goja.Callable, runtime *goja.Runtime, object *goja.Object,
	property string, value interface{}, getter Getter, setter Setter) {

	arguments := make([]goja.Value, 5)
	arguments[0] = runtime.ToValue(object)
	arguments[1] = runtime.ToValue(property)
	arguments[2] = goja.Null()
	if value != nil {
		arguments[2] = runtime.ToValue(value)
	}
	arguments[3] = goja.Null()
	if getter != nil {
		arguments[3] = runtime.ToValue(getter)
	}
	arguments[4] = goja.Null()
	if setter != nil {
		arguments[4] = runtime.ToValue(setter)
	}
	_, err := callable(runtime.ToValue(callable), arguments...)
	if err != nil {
		panic(err)
	}
}

func DeepFreezeObject(value goja.Value, runtime *goja.Runtime) goja.Value {
	var deepFreeze goja.Callable
	function := runtime.Get("deepFreeze")
	runtime.ExportTo(function, &deepFreeze)

	if val, err := deepFreeze(function, value); err != nil {
		panic(err)
	} else {
		return val
	}
}

func _freezeObject(value goja.Value, runtime *goja.Runtime) goja.Value {
	object := runtime.Get("Object").ToObject(runtime)

	var freeze goja.Callable
	object.Get("freeze")
	runtime.ExportTo(object, &freeze)

	if val, err := freeze(object, value); err != nil {
		panic(err)
	} else {
		return val
	}
}

func prepareDefineProperty(runtime *goja.Runtime) goja.Callable {
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

	prog, err := compileJavascript("system::DefineProperty", source)
	if err != nil {
		panic(err)
	}

	if value, err := runtime.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var property goja.Callable
		runtime.ExportTo(value, &property)
		return property
	}
}

func prepareDefineConstant(runtime *goja.Runtime) set_constant {
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

	prog, err := compileJavascript("system::DefineConstant", source)
	if err != nil {
		panic(err)
	}

	if value, err := runtime.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var constant set_constant
		runtime.ExportTo(value, &constant)
		return constant
	}
}

func preparePropertyDescriptor(runtime *goja.Runtime) get_property {
	source := `
	(function() {
		return function(object, set_property) {
			return Object.getOwnPropertyDescriptor(object, set_property)
		}
	})()
	`

	prog, err := compileJavascript("system::PropertyDescriptor", source)
	if err != nil {
		panic(err)
	}

	if value, err := runtime.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var property get_property
		runtime.ExportTo(value, &property)
		return property
	}
}
