package sbgoja

import (
	"github.com/relationsone/gomini"
	"github.com/dop251/goja"
	"reflect"
	"github.com/apex/log"
	"github.com/dop251/goja/parser"
	"fmt"
	"github.com/go-errors/errors"
)

var (
	typeJsCallable      = reflect.TypeOf((*gomini.Callable)(nil)).Elem()
	typeJsCallableArray = reflect.TypeOf([]gomini.Callable{})

	typeJsObject      = reflect.TypeOf((*gomini.Object)(nil)).Elem()
	typeJsObjectArray = reflect.TypeOf([]gomini.Object{})

	typeJsValue      = reflect.TypeOf((*gomini.Value)(nil)).Elem()
	typeJsValueArray = reflect.TypeOf([]gomini.Value{})

	typeGojaCallable         = reflect.TypeOf((*goja.Callable)(nil)).Elem()
	typeGojaCallableExploded = reflect.TypeOf((*func(goja.Value, ...goja.Value) (goja.Value, error))(nil)).Elem()

	typeGojaCallableArray         = reflect.TypeOf([]goja.Callable{})
	typeGojaCallableArrayExploded = reflect.TypeOf([]func(goja.Value, ...goja.Value) (goja.Value, error){})

	typeGojaObject      = reflect.TypeOf(&goja.Object{})
	typeGojaObjectArray = reflect.TypeOf([]*goja.Object{})

	typeGojaValue      = reflect.TypeOf((*goja.Value)(nil)).Elem()
	typeGojaValueArray = reflect.TypeOf([]goja.Value{})
)

func NewSandbox(bundle gomini.Bundle) gomini.Sandbox {
	runtime := goja.New()
	sandbox := &sandbox{
		runtime: runtime,
		bundle:  bundle,
	}
	sandbox.null = &_object{
		_value: &_value{
			value:   goja.Null(),
			sandbox: sandbox,
		},
	}
	sandbox.undefined = &_object{
		_value: &_value{
			value:   goja.Undefined(),
			sandbox: sandbox,
		},
	}
	sandbox.global = newJsObject(runtime.GlobalObject(), sandbox)

	sandbox.deepfreeze = prepareDeepFreeze(runtime)
	sandbox.securityproxy = newSecurityProxy(sandbox)

	return sandbox
}

type sandbox struct {
	runtime *goja.Runtime
	bundle  gomini.Bundle

	null      gomini.Value
	undefined gomini.Value
	global    gomini.Object

	deepfreeze    func(object *goja.Object)
	securityproxy *securityProxy
}

func (s *sandbox) NewObject() gomini.Object {
	jsObject := s.runtime.NewObject()
	return newJsObject(jsObject, s)
}

func (s *sandbox) NewObjectCreator(objectName string) gomini.ObjectCreator {
	return newObjectCreator(objectName, s)
}

func (s *sandbox) NewNamedNativeFunction(functionName string, function gomini.GoFunction) gomini.Value {
	return newJsValue(s.runtime.NewNamedNativeFunction(functionName, function), s)
}

func (s *sandbox) NewTypeError(args ...interface{}) gomini.Object {
	return newJsObject(s.runtime.NewTypeError(args), s)
}

func (s *sandbox) NewError(err error) gomini.Object {
	return newJsObject(s.runtime.NewGoError(err), s)
}

func (s *sandbox) NewModuleProxy(object gomini.Object, objectName string, caller gomini.Bundle) (gomini.Object, error) {
	proxy, err := s.securityproxy.makeProxy(unwrapGojaObject(object), objectName, s.bundle, caller)
	if err != nil {
		return nil, err
	}
	return newJsObject(proxy, s), nil
}

func (s *sandbox) IsAccessible(module gomini.Module, caller gomini.Bundle) error {
	property := module.Name() + ".inject"
	return sandboxSecurityCheck(property, s.bundle, caller)
}

func (s *sandbox) Compile(filename, source string) (gomini.Script, bool, error) {
	ast, err := parser.ParseFile(nil, filename, source, 0)
	if err != nil {
		return nil, false, err
	}

	prog, err := goja.CompileAST(ast, true)
	if err != nil {
		return nil, false, err
	}
	return prog, true, nil
}

func (s *sandbox) Execute(script gomini.Script) (gomini.Value, error) {
	value, err := s.runtime.RunProgram(script.(*goja.Program))
	if err != nil {
		return nil, err
	}
	return newJsValue(value, s), nil
}

func (s *sandbox) CaptureCallStack(maxStackFrames int) []gomini.StackFrame {
	sf := s.runtime.CaptureCallStack(maxStackFrames)
	stackFrames := make([]gomini.StackFrame, len(sf))
	for i, stackFrame := range sf {
		stackFrames[i] = _stackFrame{
			original: stackFrame,
		}
	}
	return stackFrames
}

func (s *sandbox) NewDebugger() (interface{}, error) {
	// TODO
	return nil, nil
}

func (s *sandbox) NullValue() gomini.Value {
	return s.null
}

func (s *sandbox) UndefinedValue() gomini.Value {
	return s.undefined
}

func (s *sandbox) Global() gomini.Object {
	return s.global
}

func (s *sandbox) ToValue(value interface{}) gomini.Value {
	return newJsValue(s.runtime.ToValue(value), s)
}

func (s *sandbox) Export(value gomini.Value, target interface{}) error {
	val := unwrapGojaValue(value)

	v := reflect.ValueOf(target)
	for ; v.Kind() == reflect.Ptr; {
		v = reflect.Indirect(v)
	}

	typ := v.Type()
	switch typ {
	case typeJsObject:
		if value.IsObject() {
			target = value.(gomini.Object)
		}
		target = newJsObject(val.ToObject(s.runtime), s)

	case typeJsObjectArray:
		array := val.Export().([]*goja.Object)
		a := make([]gomini.Object, len(array))
		for i, o := range array {
			a[i] = newJsObject(o, s)
		}
		target = a

	case typeJsValue:
		target = value

	case typeJsValueArray:
		array := val.Export().([]goja.Value)
		a := make([]gomini.Value, len(array))
		for i, o := range array {
			a[i] = newJsValue(o, s)
		}
		target = a

	case typeGojaObject:
		if value.IsObject() {
			target = unwrapGojaObject(value.(gomini.Object))
		}
		target = val.ToObject(s.runtime)

	case typeGojaValue:
		target = unwrapGojaValue(value)

	case typeJsCallableArray:
		var array []goja.Value
		if err := s.runtime.ExportTo(val, &array); err != nil {
			return err
		}
		a := make([]gomini.Callable, len(array))
		for i, o := range array {
			var callable goja.Callable
			s.runtime.ExportTo(o, &callable)
			a[i] = newGojaCallableAdapter(callable, s)
		}
		v.Set(reflect.ValueOf(a))

	case typeJsCallable:
		var callable goja.Callable
		if err := s.runtime.ExportTo(val, &callable); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(newGojaCallableAdapter(callable, s)))

	default:
		switch typ.Kind() {
		case reflect.Func:
			if err := adaptJsFunction(target, val, s); err != nil {
				return err
			}

		default:
			return s.runtime.ExportTo(val, target)
		}
	}

	return nil
}

func (s *sandbox) gojaFreeze(object *goja.Object) {
	freezeObject(object, s.runtime)
}

func (s *sandbox) gojaDeepFreeze(object *goja.Object) {
	s.deepfreeze(object)
}

func isArray(value gomini.Value) bool {
	kind := value.ExportType().Kind()
	return isDefined(value) && kind == reflect.Slice || kind == reflect.Array
}

func isDefined(value gomini.Value) bool {
	v := unwrapValue(value)
	return v != goja.Undefined() && v != goja.Null()
}

func unwrapValue(value interface{}) interface{} {
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
	case gomini.Value:
		return t.Unwrap().(goja.Value)
	}
	return value
}

func unwrapGojaValue(value gomini.Value) goja.Value {
	return value.Unwrap().(goja.Value)
}

func unwrapGojaObject(value gomini.Object) *goja.Object {
	if t, ok := value.Unwrap().(*goja.Object); ok {
		return t
	}
	panic("illegal parameter, value is not an object")
}

func adaptJsNativeFunction(function gomini.NativeFunction, sandbox *sandbox) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		this := newJsValue(call.This, sandbox)
		arguments := make([]gomini.Value, len(call.Arguments))
		for i, arg := range call.Arguments {
			arguments[i] = newJsValue(arg, sandbox)
		}
		ret := function(gomini.FunctionCall{
			This:      this,
			Arguments: arguments,
			Bundle:    sandbox.bundle,
		})
		if ret == nil || ret == sandbox.NullValue() {
			return goja.Null()
		}
		if ret == sandbox.UndefinedValue() {
			return goja.Undefined()
		}
		return unwrapGojaValue(ret)
	}
}

func adaptGoFunction(functionName string, function gomini.GoFunction, value reflect.Value, sandbox *sandbox) func(goja.FunctionCall) goja.Value {
	adapterFunction := makeAdapterFunction(function, value, sandbox)
	v := sandbox.runtime.NewNamedNativeFunction(functionName, adapterFunction)
	return v.Export().(func(goja.FunctionCall) goja.Value)
}

func adaptJsFunction(target interface{}, value goja.Value, sandbox *sandbox) error {
	vt := reflect.ValueOf(target)
	for ; vt.Kind() == reflect.Ptr; {
		vt = reflect.Indirect(vt)
	}

	ft := vt.Type()
	newFt := makeGojaMethodSignature(ft)
	adapted := ft != newFt

	if adapted {
		fprtvalue := reflect.New(newFt)
		fvalue := reflect.Indirect(fprtvalue)

		functionPtr := fprtvalue.Interface()
		if err := sandbox.runtime.ExportTo(value, functionPtr); err != nil {
			return err
		}

		t := makeAdapterFunction(fvalue.Interface(), fvalue, sandbox)
		vt.Set(reflect.ValueOf(t))
		return nil
	}

	return sandbox.runtime.ExportTo(value, target)
}

func makeAdapterFunction(function interface{}, value reflect.Value, sandbox *sandbox) interface{} {
	ft := reflect.TypeOf(function)

	if ft == typeGojaCallableExploded {
		return newCallableAdapter(function, sandbox)
	} else if ft == typeGojaCallable {
		return function
	}

	adapted := makeGojaMethodSignature(ft)
	if ft == adapted {
		return function
	}
	log.Debugf("Sandbox: Adapter function signature: %s ==> %s", ft.String(), adapted.String())
	return reflect.MakeFunc(adapted, adapterFunction(value, adapted.IsVariadic(), sandbox)).Interface()
}

func newCallableAdapter(function interface{}, sandbox *sandbox) gomini.Callable {
	return func(this gomini.Value, arguments ...gomini.Value) (gomini.Value, error) {
		f := function.(func(goja.Value, ...goja.Value) (goja.Value, error))
		args := make([]goja.Value, len(arguments))
		for i, arg := range arguments {
			args[i] = unwrapGojaValue(arg)
		}
		v, err := f(unwrapGojaValue(this), args...)
		if err != nil {
			return nil, err
		}
		return newJsValue(v, sandbox), nil
	}
}

func newGojaCallableAdapter(function goja.Callable, sandbox *sandbox) gomini.Callable {
	return func(this gomini.Value, arguments ...gomini.Value) (gomini.Value, error) {
		args := make([]goja.Value, len(arguments))
		for i, arg := range arguments {
			args[i] = unwrapGojaValue(arg)
		}
		v, err := function(unwrapGojaValue(this), args...)
		if err != nil {
			return nil, err
		}
		return newJsValue(v, sandbox), nil
	}
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
		case typeJsObjectArray:
			p = typeGojaObjectArray
			adapted = true
		case typeJsValue:
			p = typeGojaValue
			adapted = true
		case typeJsValueArray:
			p = typeGojaValueArray
			adapted = true
		case typeGojaObject:
			p = typeJsObject
			adapted = true
		case typeGojaObjectArray:
			p = typeJsObjectArray
			adapted = true
		case typeGojaValue:
			p = typeJsValue
			adapted = true
		case typeGojaValueArray:
			p = typeJsValueArray
			adapted = true

		default:
			switch p.Kind() {
			case reflect.Func:
				log.Debugf("Sandbox: Adapting parameter types for: %s", p.String())
				temp := makeGojaMethodSignature(p)
				if temp != p {
					log.Debugf("Sandbox: Adapter function signature: %s ==> %s", p.String(), temp.String())
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
		case typeGojaObjectArray:
			o = typeJsObjectArray
			adapted = true
		case typeGojaValue:
			o = typeJsValue
			adapted = true
		case typeGojaValueArray:
			o = typeJsValueArray
			adapted = true
		case typeJsObject:
			o = typeGojaObject
			adapted = true
		case typeJsObjectArray:
			o = typeGojaObjectArray
			adapted = true
		case typeJsValue:
			o = typeGojaValue
			adapted = true
		case typeJsValueArray:
			o = typeGojaValueArray
			adapted = true

		default:
			switch o.Kind() {
			case reflect.Func:
				log.Debugf("Sandbox: Adapting return types for: %s", o.String())
				temp := makeGojaMethodSignature(o)
				if temp != o {
					log.Debugf("Sandbox: Adapter function signature: %s ==> %s", o.String(), temp.String())
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

func adapterFunction(value reflect.Value, variadic bool, sandbox *sandbox) func([]reflect.Value) []reflect.Value {
	return func(args []reflect.Value) []reflect.Value {
		params := make([]reflect.Value, 0)
		for _, arg := range args {
			switch t := arg.Interface().(type) {
			case gomini.Object:
				var a reflect.Value
				if t == sandbox.UndefinedValue() {
					a = reflect.ValueOf(goja.Undefined())
				} else if t == sandbox.NullValue() {
					a = reflect.ValueOf(goja.Null())
				} else {
					a = reflect.ValueOf(unwrapGojaObject(t))
				}
				params = append(params, a)

			case []gomini.Object:
				for _, o := range t {
					params = append(params, reflect.ValueOf(unwrapGojaObject(o)))
				}

			case gomini.Value:
				params = append(params, reflect.ValueOf(unwrapGojaValue(t)))

			case []gomini.Value:
				for _, o := range t {
					params = append(params, reflect.ValueOf(unwrapGojaValue(o)))
				}

			case *goja.Object:
				params = append(params, reflect.ValueOf(newJsObject(t, sandbox)))

			case []*goja.Object:
				for _, o := range t {
					params = append(params, reflect.ValueOf(newJsObject(o, sandbox)))
				}

			case goja.Value:
				params = append(params, reflect.ValueOf(newJsValue(t, sandbox)))

			case []goja.Value:
				for _, o := range t {
					params = append(params, reflect.ValueOf(newJsValue(o, sandbox)))
				}

			default:
				switch arg.Type().Kind() {
				case reflect.Func:
					log.Debugf("Sandbox: Function parameter found: %s", arg.String())
					params = append(params, reflect.ValueOf(makeAdapterFunction(arg.Interface(), arg, sandbox)))

				default:
					params = append(params, arg)
				}
			}
		}

		log.Debugf("Sandbox: Calling function: %s", value.String())
		var ret []reflect.Value
		if variadic && len(params) > 2 {
			ret = value.CallSlice(params)
		} else {
			ret = value.Call(params)
		}

		for i, r := range ret {
			switch r.Type() {
			case typeJsObject:
				v := r.Interface()
				if v == nil {
					ret[i] = reflect.ValueOf(goja.Null())
				} else {
					ret[i] = reflect.ValueOf(unwrapGojaObject(v.(gomini.Object))).Elem()
				}

			case typeJsObjectArray:
				array := r.Interface().([]gomini.Object)
				a := make([]*goja.Object, len(array))
				for i, o := range array {
					a[i] = unwrapGojaObject(o)
				}
				ret[i] = reflect.ValueOf(a)

			case typeJsValue:
				v := r.Interface()
				if v == nil {
					ret[i] = reflect.ValueOf(goja.Null())
				} else {
					ret[i] = reflect.ValueOf(unwrapGojaValue(v.(gomini.Value))).Elem()
				}

			case typeJsValueArray:
				array := r.Interface().([]gomini.Value)
				a := make([]goja.Value, len(array))
				for i, o := range array {
					a[i] = unwrapGojaValue(o)
				}
				ret[i] = reflect.ValueOf(a)

			case typeGojaObject:
				v := r.Interface()
				if v == nil {
					ret[i] = wrapToInterface(sandbox.NullValue(), typeJsObject)
				} else {
					ret[i] = wrapToInterface(newJsObject(v.(*goja.Object), sandbox), typeJsObject)
				}

			case typeGojaObjectArray:
				array := r.Interface().([]*goja.Object)
				a := make([]gomini.Object, len(array))
				for i, o := range array {
					a[i] = newJsObject(o, sandbox)
				}
				ret[i] = reflect.ValueOf(a)

			case typeGojaValue:
				v := r.Interface()
				if v == nil {
					ret[i] = wrapToInterface(sandbox.NullValue(), typeJsValue)
				} else {
					ret[i] = wrapToInterface(newJsValue(v.(goja.Value), sandbox), typeJsValue)
				}

			case typeGojaValueArray:
				array := r.Interface().([]goja.Value)
				a := make([]gomini.Value, len(array))
				for i, o := range array {
					a[i] = newJsValue(o, sandbox)
				}
				ret[i] = reflect.ValueOf(a)

			default:
				switch r.Type().Kind() {
				case reflect.Func:
					log.Debugf("Sandbox: Function return value found: %s", r.String())
					ret[i] = reflect.ValueOf(makeAdapterFunction(r.Interface(), r, sandbox))
				}
			}
		}

		return ret
	}
}

func wrapToInterface(val interface{}, returnType reflect.Type) reflect.Value {
	value := reflect.ValueOf(val)
	typ := value.Type()
	log.Debugf("Sandbox: Type1: %s, Type2: %s", typ.String(), returnType.String())
	if typ == returnType || typ.Implements(returnType) {
		return value.Convert(returnType)
	}
	return value
}

func freezeObject(value goja.Value, runtime *goja.Runtime) goja.Value {
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

func sandboxSecurityCheck(property string, origin gomini.Bundle, caller gomini.Bundle) error {
	interceptor := origin.SecurityInterceptor()
	if !caller.Privileged() && interceptor != nil {
		if !interceptor(caller, property) {
			msg := fmt.Sprintf("illegal access violation: %s cannot access %s::%s", caller.Name(), origin.Name(), property)
			return errors.New(msg)
		}
	}
	log.Debugf("SecurityProxy: SecurityInterceptor check success: %s", property)
	return nil
}

func unwrapGojaRuntime(bundle gomini.Bundle) *goja.Runtime {
	return bundle.Sandbox().(*sandbox).runtime
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

	value, err := runtime.RunScript("system::DeepFreeze", source)
	if err != nil {
		panic(err)
	}
	var deepFreeze func(*goja.Object)
	runtime.ExportTo(value, &deepFreeze)
	return deepFreeze
}
