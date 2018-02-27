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

func adaptJsNativeFunction(function JsNativeFunction, bundle Bundle) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		this := newJsValue(call.This, bundle)
		arguments := make([]JsValue, len(call.Arguments))
		for i, arg := range call.Arguments {
			arguments[i] = newJsValue(arg, bundle)
		}
		ret := function(JsFunctionCall{
			This:      this,
			Arguments: arguments,
			Bundle:    bundle,
		})
		if ret == nil || ret == bundle.Null() {
			return goja.Null()
		}
		if ret == bundle.Undefined() {
			return goja.Undefined()
		}
		return ret.unwrap()
	}
}

func adaptGoFunction(functionName string, function JsGoFunction, value reflect.Value, bundle Bundle) func(call goja.FunctionCall) goja.Value {
	adapterFunction := makeAdapterFunction(function, value, bundle)
	v := bundle.Sandbox().NewNamedNativeFunction(functionName, adapterFunction)
	return v.Export().(func(goja.FunctionCall) goja.Value)
}

func makeAdapterFunction(function interface{}, value reflect.Value, bundle Bundle) interface{} {
	ft := reflect.TypeOf(function)
	adapted := makeGojaMethodSignature(ft)
	if ft == adapted {
		return function
	}
	log.Debugf("Kernel: Adapter function signature: %s ==> %s", ft.String(), adapted.String())
	return reflect.MakeFunc(adapted, adapterFunction(value, adapted.IsVariadic(), bundle)).Interface()
}

func makeGojaMethodSignature(ft reflect.Type) reflect.Type {
	if ft.Kind() != reflect.Func {
		panic("illegal type, function expected, but got " + ft.String())
	}

	numIn := ft.NumIn()
	numOut := ft.NumOut()

	variadic := ft.IsVariadic()

	adapted := false

	in := make([]reflect.Type, numIn)
	for i := 0; i < numIn; i++ {
		p := ft.In(i)
		switch p {
		case typeJsObject:
			p = typeGojaObject
			adapted = true
		case typeJsValue:
			p = typeGojaValue
			adapted = true
		case typeGojaObject:
			p = typeJsObject
			adapted = true
		case typeGojaValue:
			p = typeJsValue
			adapted = true

		default:
			switch p.Kind() {
			case reflect.Func:
				log.Debugf("Kernel: Adapting parameter types for: %s", p.String())
				temp := makeGojaMethodSignature(p)
				if temp != p {
					log.Debugf("Kernel: Adapter function signature: %s ==> %s", p.String(), temp.String())
					p = temp
					adapted = true
				}
			}
		}
		in[i] = p
	}

	out := make([]reflect.Type, numOut)
	for i := 0; i < numOut; i++ {
		o := ft.Out(i)
		switch o {
		case typeGojaObject:
			o = typeJsObject
			adapted = true
		case typeGojaValue:
			o = typeJsValue
			adapted = true
		case typeJsObject:
			o = typeGojaObject
			adapted = true
		case typeJsValue:
			o = typeGojaValue
			adapted = true

		default:
			switch o.Kind() {
			case reflect.Func:
				log.Debugf("Kernel: Adapting return types for: %s", o.String())
				temp := makeGojaMethodSignature(o)
				if temp != o {
					log.Debugf("Kernel: Adapter function signature: %s ==> %s", o.String(), temp.String())
					o = temp
					adapted = true
				}
			}
		}
		out[i] = o
	}

	if !adapted {
		return ft
	}

	return reflect.FuncOf(in, out, variadic)
}

func adapterFunction(value reflect.Value, variadic bool, bundle Bundle) func([]reflect.Value) []reflect.Value {
	return func(args []reflect.Value) []reflect.Value {
		for i, arg := range args {
			switch t := arg.Interface().(type) {
			case JsValue:
				args[i] = reflect.ValueOf(t.unwrap())
			case *goja.Object:
				args[i] = reflect.ValueOf(newJsObject(t, bundle))
			case goja.Value:
				args[i] = reflect.ValueOf(newJsValue(t, bundle))

			default:
				switch arg.Type().Kind() {
				case reflect.Func:
					log.Debugf("Kernel: Function parameter found: %s", arg.String())
					args[i] = reflect.ValueOf(makeAdapterFunction(arg.Interface(), arg, bundle))
				}
			}
		}

		log.Debugf("Kernel: Calling function: %s", value.String())
		var ret []reflect.Value
		if variadic {
			ret = value.CallSlice(args)
		} else {
			ret = value.Call(args)
		}

		for i, r := range ret {
			switch r.Type() {
			case typeJsObject:
				ret[i] = reflect.ValueOf(r.Interface().(JsObject).unwrap())
			case typeJsValue:
				ret[i] = reflect.ValueOf(r.Interface().(JsValue).unwrap()).Elem()
			case typeGojaObject:
				ret[i] = wrapToInterface(newJsObject(r.Interface().(*goja.Object), bundle), typeJsObject)
			case typeGojaValue:
				ret[i] = wrapToInterface(newJsValue(r.Interface().(goja.Value), bundle), typeJsValue)

			default:
				switch r.Type().Kind() {
				case reflect.Func:
					log.Debugf("Kernel: Function return value found: %s", r.String())
					ret[i] = reflect.ValueOf(makeAdapterFunction(r.Interface(), r, bundle))
				}
			}
		}

		return ret
	}
}

func wrapToInterface(val interface{}, returnType reflect.Type) reflect.Value {
	value := reflect.ValueOf(val)
	typ := value.Type()
	log.Debugf("Kernel: Type1: %s, Type2: %s", typ.String(), returnType.String())
	if typ == returnType || typ.Implements(returnType) {
		return value.Convert(returnType)
	}
	return value
}

func unwrapValue(value Any) Any {
	x := reflect.ValueOf(value)
	switch x.Kind() {
	case reflect.String:
		return x.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return x.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return x.Uint()
	case reflect.Float32, reflect.Float64:
		return x.Float()
	case reflect.Bool:
		return x.Bool()
	}

	switch t := value.(type) {
	case JsValue:
		return t.unwrap()
	}
	return value
}

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

func isJavaScript(filename string) bool {
	return strings.HasSuffix(filename, ".js") ||
		strings.HasSuffix(filename, ".js.gz") ||
		strings.HasSuffix(filename, ".js.bz2")
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

func __freezeObject(value goja.Value, runtime *goja.Runtime) goja.Value {
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
