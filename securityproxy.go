package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"fmt"
)

type _propertydescriptor struct {
	value        interface{}
	writable     bool
	get          Getter
	set          Setter
	configurable bool
	enumerable   bool
}

type _adaptnull func(property string) error
type _adaptfunction func(property string, prop goja.Value, call goja.Callable, construct goja.Callable) error
type _adaptarray func(property string, array []goja.Value) error
type _adaptobject func(property string, object goja.Value) error
type _adaptproperty func(property string, descriptor _propertydescriptor)
type adapter_function func(value interface{}, adaptnull _adaptnull, adaptfunction _adaptfunction,
	adaptarray _adaptarray, adaptobject _adaptobject, adaptproperty _adaptproperty) error

type securityProxy struct {
	vm           *goja.Runtime
	adaptingCall adapter_function
}

func newSecurityProxy(kernel *kernel, bundle Bundle, basePath string) (*securityProxy, error) {
	filename := findScriptFile("js/kernel/securityProxy.js", basePath)
	source, err := kernel.loadSource(filename)
	if err != nil {
		return nil, errors.New(err)
	}

	value, err := prepareJavascript(filename, source, bundle.getSandbox())
	if err != nil {
		return nil, errors.New(err)
	}

	var adaptingCall adapter_function
	err = bundle.getSandbox().ExportTo(value, &adaptingCall)
	if err != nil {
		return nil, err
	}

	return &securityProxy{
		vm:           bundle.getSandbox(),
		adaptingCall: adaptingCall,
	}, nil
}

func (s *securityProxy) adapt(source, target *goja.Object, origin Bundle, caller Bundle) error {
	var adaptnull _adaptnull = func(property string) error {
		return target.Set(property, goja.Null())
	}

	var adaptfunction _adaptfunction = func(property string, prop goja.Value, function goja.Callable, constructor goja.Callable) error {
		call := func(call goja.FunctionCall) goja.Value {
			s.sandboxSecurityCheck(property, origin, caller)

			parameters := make([]goja.Value, len(call.Arguments))
			for i, arg := range call.Arguments {
				parameters[i] = origin.ToValue(arg.Export())
			}

			proxy, err := function(source, parameters...)
			if err != nil {
				panic(err)
			}

			return caller.ToValue(proxy.Export())
		}

		construct := func(args []goja.Value) *goja.Object {
			s.sandboxSecurityCheck(property, origin, caller)

			arguments := make([]goja.Value, len(args))
			for i, arg := range args {
				arguments[i] = origin.ToValue(arg.Export())
			}

			proxy, err := constructor(origin.ToValue(constructor), arguments...)
			if err != nil {
				panic(err)
			}

			instance := proxy.ToObject(origin.getSandbox())
			newObject := caller.NewObject()
			if err := s.adapt(instance, newObject, origin, caller); err != nil {
				panic(err)
			}

			return newObject
		}

		proxy := caller.getSandbox().CreateFunctionProxy(call, construct)
		return target.Set(property, proxy)
	}

	var adaptarray _adaptarray = func(property string, array []goja.Value) error {
		values := make([]interface{}, len(array))

		for i, value := range array {
			values[i] = caller.ToValue(value.Export())
		}

		return target.Set(property, values)
	}

	var adaptobject _adaptobject = func(property string, object goja.Value) error {
		t := caller.NewObject()

		obj := object.ToObject(origin.getSandbox())
		err := s.adapt(obj, t, origin, caller)
		if err != nil {
			return err
		}

		return target.Set(property, obj)
	}

	var adaptproperty _adaptproperty = func(property string, descriptor _propertydescriptor) {
		value := descriptor.value
		getter := descriptor.get
		setter := descriptor.set

		var get = func() interface{} {
			s.sandboxSecurityCheck(property, origin, caller)
			if getter != nil {
				return caller.ToValue(getter())
			}
			return caller.ToValue(value)
		}

		var set Setter = nil
		if descriptor.writable {
			set = func(value interface{}) {
				if setter != nil {
					s.sandboxSecurityCheck(property, origin, caller)
					setter(value)
				}
			}
		}

		caller.DefineProperty(target, property, nil, get, set)
	}

	return s.adaptingCall(source, adaptnull, adaptfunction, adaptarray, adaptobject, adaptproperty)
}

func (s *securityProxy) sandboxSecurityCheck(property string, origin Bundle, caller Bundle) {
	interceptor := origin.SecurityInterceptor()
	if !caller.Privileged() && interceptor != nil {
		if !interceptor(caller, property) {
			msg := fmt.Sprintf("Illegal access violation: %s cannot access %s::%s",
				caller.Name(), origin.Name(), property)
			panic(errors.New(msg))
		}
	}
	fmt.Println(fmt.Sprintf("SecurityProxy: SecurityInterceptor check success: %s", property))
}
