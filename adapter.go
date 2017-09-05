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

type adapter struct {
	vm           *goja.Runtime
	adaptingCall adapter_function
}

func newAdapter(kernel *kernel, module Module) (*adapter, error) {
	filename := findScriptFile("js/kernel/adapter.js", kernel.baseDir)
	source, err := kernel.loadSource(filename)
	if err != nil {
		return nil, errors.New(err)
	}

	value, err := prepareJavascript(filename, source, module.getVm())
	if err != nil {
		return nil, errors.New(err)
	}

	var adaptingCall adapter_function
	//var adaptingCall goja.Callable
	err = module.getVm().ExportTo(value, &adaptingCall)
	if err != nil {
		return nil, err
	}

	return &adapter{
		vm:           module.getVm(),
		adaptingCall: adaptingCall,
	}, nil
}

func (adapter *adapter) adapt(source, target *goja.Object, origin Module, caller Module) error {
	var adaptnull _adaptnull = func(property string) error {
		return target.Set(property, goja.Null())
	}

	var adaptfunction _adaptfunction = func(property string, prop goja.Value, function goja.Callable, constructor goja.Callable) error {
		call := func(call goja.FunctionCall) goja.Value {
			if property == "registerRequestHandler" {
				fmt.Println("BÄM")
			}

			sandboxSecurityCheck(property, origin, caller)

			parameters := make([]goja.Value, len(call.Arguments))
			for i, arg := range call.Arguments {
				parameters[i] = origin.ToValue(arg.Export())
			}

			s, err := function(source, parameters...)
			if err != nil {
				panic(err)
			}

			return caller.ToValue(s.Export())
		}

		construct := func(args []goja.Value) *goja.Object {
			sandboxSecurityCheck(property, origin, caller)

			arguments := make([]goja.Value, len(args))
			for i, arg := range args {
				arguments[i] = origin.ToValue(arg.Export())
			}

			s, err := constructor(origin.ToValue(constructor), arguments...)
			if err != nil {
				panic(err)
			}

			instance := s.ToObject(origin.getVm())
			newObject := caller.NewObject()
			if err := adapter.adapt(instance, newObject, origin, caller); err != nil {
				panic(err)
			}

			return newObject
		}

		proxy := caller.getVm().CreateFunctionProxy(call, construct)
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

		obj := object.ToObject(origin.getVm())
		err := adapter.adapt(obj, t, origin, caller)
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
			sandboxSecurityCheck(property, origin, caller)
			if getter != nil {
				return caller.ToValue(getter())
			}
			return caller.ToValue(value)
		}

		var set Setter = nil
		if descriptor.writable {
			set = func(value interface{}) {
				if setter != nil {
					sandboxSecurityCheck(property, origin, caller)
					setter(value)
				}
			}
		}

		caller.DefineProperty(target, property, nil, get, set)
	}

	return adapter.adaptingCall(source, adaptnull, adaptfunction, adaptarray, adaptobject, adaptproperty)
}
