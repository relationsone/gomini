package gomini

import (
	"github.com/dop251/goja"
	"reflect"
	"os"
	"strings"
	"crypto/sha256"
	"encoding/hex"
	"github.com/dop251/goja/parser"
	"github.com/spf13/afero"
	"fmt"
	"github.com/go-errors/errors"
	"github.com/apex/log"
)

type get_property func(object *goja.Object, propertyName string) goja.Value

func isArray(value goja.Value) bool {
	kind := value.ExportType().Kind()
	return isDefined(value) && kind == reflect.Slice || kind == reflect.Array
}

func isDefined(value goja.Value) bool {
	return value != goja.Null() && value != goja.Undefined()
}

func fileExists(filesystem afero.Fs, filename string) bool {
	if _, err := filesystem.Stat(filename); err != nil {
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

func loadPlainJavascript(kernel *kernel, filename string, loader, target Bundle) (goja.Value, error) {
	scriptPath := kernel.resolveScriptPath(loader, filename)
	if prog, err := kernel.loadScriptSource(scriptPath, true); err != nil {
		return nil, err
	} else {
		return executeJavascript(prog, target)
	}
}

func prepareJavascript(filename string, source string, bundle Bundle) (goja.Value, error) {
	if prog, err := compileJavascript(filename, source); err != nil {
		return nil, err
	} else {
		return executeJavascript(prog, bundle)
	}
}

func tsCacheFilename(path string, bundle Bundle, kernel *kernel) string {
	kernelBasedPath := kernel.toKernelPath(path, bundle)
	return hash(kernelBasedPath)
}

func isCachedFileCurrent(path string, bundle Bundle) bool {
	return false
}

func executeJavascript(prog *goja.Program, bundle Bundle) (goja.Value, error) {
	return bundle.Sandbox().RunProgram(prog)
}

func sandboxSecurityCheck(property string, origin Bundle, caller Bundle) {
	interceptor := origin.SecurityInterceptor()
	if !caller.Privileged() && interceptor != nil {
		if !interceptor(caller, property) {
			msg := fmt.Sprintf("illegal access violation: %s cannot access %s::%s", caller.Name(), origin.Name(), property)
			panic(errors.New(msg))
		}
	}
	log.Debugf("SecurityProxy: SecurityInterceptor check success: %s", property)
}

func compileJavascript(filename string, source string) (*goja.Program, error) {
	// TODO Is this still necessary?
	/*if !strings.HasPrefix(filename, "system::") && !filepath.IsAbs(filename) {
		return nil, fmt.Errorf("provided path is not absolute: %s", filename)
	}*/
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

func prepareDeepFreeze(runtime *goja.Runtime) func(*goja.Object) {
	source := `
	(function () {
    	return function (o) {
        	Object.freeze(o);
	        Object.getOwnPropertyNames(o).forEach(function (prop) {
    	        if (o.hasOwnProperty(prop)
        	        && o[prop] !== null
            	    && (typeof o[prop] === "object" || typeof o[prop] === "function")
                	&& !Object.isFrozen(o[prop])) {
	                deepFreeze(o[prop]);
    	        }
        	});
	        return o;
    	};
	})();
	`

	prog, err := compileJavascript("system::DeepFreeze", source)
	if err != nil {
		panic(err)
	}

	if value, err := runtime.RunProgram(prog); err != nil {
		panic(err)
	} else {
		var deepFreeze func(*goja.Object)
		runtime.ExportTo(value, &deepFreeze)
		return deepFreeze
	}
}
