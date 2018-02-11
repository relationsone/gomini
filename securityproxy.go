package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"fmt"
	"reflect"
	"github.com/apex/log"
)

const jsSecurityProxy = "/js/kernel/securityProxy.js"

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

func newSecurityProxy(kernel *kernel, bundle Bundle) (*securityProxy, error) {
	value, err := loadPlainJavascript(kernel, jsSecurityProxy, kernel, bundle)
	if err != nil {
		return nil, errors.New(err)
	}

	sandbox := bundle.getSandbox()

	var adaptingCall adapter_function
	err = sandbox.ExportTo(value, &adaptingCall)
	if err != nil {
		return nil, err
	}

	return &securityProxy{
		vm:           sandbox,
		adaptingCall: adaptingCall,
	}, nil
}

func (s *securityProxy) makeProxy(target *goja.Object, propertyName string, origin, caller Bundle) (goja.Value, error) {
	handler := &goja.ProxyTrapConfig{
		GetPrototypeOf: func(target *goja.Object) *goja.Object {
			return caller.getSandbox().NewTypeError("Proxies have no prototypes")
		},
		IsExtensible: func(target *goja.Object) bool {
			return false
		},
		DefineProperty: func(target *goja.Object, key string, propertyDescriptor goja.PropertyDescriptor) bool {
			return false
		},
		DeleteProperty: func(target *goja.Object, property string) bool {
			return false
		},
		PreventExtensions: func(target *goja.Object) bool {
			return true
		},
		Set: func(target *goja.Object, property string, value goja.Value, receiver *goja.Object) bool {
			return false
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, prop string) goja.PropertyDescriptor {
			return goja.PropertyDescriptor{
				Writable:     goja.FLAG_FALSE,
				Configurable: goja.FLAG_FALSE,
				Enumerable:   goja.FLAG_TRUE,
				Value:        nil,
				Setter:       nil,
				Getter: caller.ToValue(func() goja.Value {
					return nil
				}),
			}
		},
		Get: func(target *goja.Object, property string, receiver *goja.Object) goja.Value {
			s.sandboxSecurityCheck(propertyName + "." + property + ".get", origin, caller)

			source := target.Get(property)
			if o, ok := source.(*goja.Object); ok {
				proxy, err := s.makeProxy(o, propertyName + "." + property, origin, caller)
				if err != nil {
					panic(err)
				}
				return proxy
			}
			return source
		},
		Has: func(target *goja.Object, property string) bool {
			s.sandboxSecurityCheck(propertyName + "." + property + ".has", origin, caller)

			return target.Get(property) != nil
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			return caller.ToValue(target.Keys()).(*goja.Object)
		},
		Apply: func(target *goja.Object, this *goja.Object, argumentsList []goja.Value) goja.Value {
			s.sandboxSecurityCheck(propertyName + ".apply", origin, caller)

			thisProxy, err := s.makeProxy(this, propertyName + ".this", caller, origin)
			if err != nil {
				panic(err)
			}

			var function func(goja.FunctionCall) goja.Value
			err = origin.getSandbox().ExportTo(target, &function)
			if err != nil {
				panic(err)
			}

			arguments := make([]goja.Value, len(argumentsList))
			for i, arg := range argumentsList {
				if a, ok := arg.(*goja.Object); ok {
					arg, err = s.makeProxy(a, "", caller, origin)
					if err != nil {
						panic(err)
					}
				}
				arguments[i] = arg
			}

			ret := function(goja.FunctionCall{
				This:      thisProxy,
				Arguments: argumentsList, // TODO adapt arguments
			})

			return ret // TODO adapt
		},
		Construct: func(target *goja.Object, argumentsList []goja.Value, newTarget *goja.Object) *goja.Object {
			var constructor func(call goja.ConstructorCall) *goja.Object
			err := origin.getSandbox().ExportTo(target, &constructor)
			if err != nil {
				panic(err)
			}

			ret := constructor(goja.ConstructorCall{
				This:      target,
				Arguments: argumentsList, // TODO adapt arguments
			})

			proxy, err := s.makeProxy(ret, propertyName + ".constructor", origin, caller)
			if err != nil {
				panic(err)
			}

			return proxy.(*goja.Object)
		},
	}

	proxy := caller.getSandbox().NewProxy(target, handler, false, false)
	return caller.ToValue(proxy), nil
}

func (s *securityProxy) primitiveValue(value goja.Value) bool {
	switch value.ExportType().Kind() {
	case reflect.String | reflect.Int8 | reflect.Int16 | reflect.Int32 | reflect.Int64 | reflect.Int |
		reflect.Uint8 | reflect.Uint16 | reflect.Uint32 | reflect.Uint64 | reflect.Uint |
		reflect.Bool | reflect.Float32 | reflect.Float64 | reflect.Complex64 | reflect.Complex128:

		return false
	}
	return true
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

		caller.getSandbox().NewProxy(target, &goja.ProxyTrapConfig{
			Construct: func(target *goja.Object, argumentsList []goja.Value, newTarget *goja.Object) *goja.Object {
				return construct(argumentsList)
			},
			Apply: func(target *goja.Object, this *goja.Object, argumentsList []goja.Value) goja.Value {
				return nil
			},
		}, true, false)

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
			msg := fmt.Sprintf("SecurityProxy: Illegal access violation: %s cannot access %s::%s",
				caller.Name(), origin.Name(), property)
			panic(errors.New(msg))
		}
	}
	log.Infof("SecurityProxy: SecurityInterceptor check success: %s", property)
}
