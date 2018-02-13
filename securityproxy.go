package gomini

import (
	"github.com/dop251/goja"
	"github.com/go-errors/errors"
	"fmt"
	"reflect"
	"github.com/apex/log"
)

type securityProxy struct {
	vm *goja.Runtime
}

func newSecurityProxy(bundle Bundle) (*securityProxy, error) {
	return &securityProxy{
		vm: bundle.getSandbox(),
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
			s.sandboxSecurityCheck(propertyName+"."+property+".get", origin, caller)

			source := target.Get(property)
			if o, ok := source.(*goja.Object); ok {
				proxy, err := s.makeProxy(o, propertyName+"."+property, origin, caller)
				if err != nil {
					panic(err)
				}
				return proxy
			}
			return source
		},
		Has: func(target *goja.Object, property string) bool {
			s.sandboxSecurityCheck(propertyName+"."+property+".has", origin, caller)

			return target.Get(property) != nil
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			return caller.ToValue(target.Keys()).(*goja.Object)
		},
		Apply: func(target *goja.Object, this *goja.Object, argumentsList []goja.Value) goja.Value {
			s.sandboxSecurityCheck(propertyName+".apply", origin, caller)

			thisProxy, err := s.makeProxy(this, propertyName+".this", caller, origin)
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

			proxy, err := s.makeProxy(ret, propertyName+".constructor", origin, caller)
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

func (s *securityProxy) sandboxSecurityCheck(property string, origin Bundle, caller Bundle) {
	interceptor := origin.SecurityInterceptor()
	if !caller.Privileged() && interceptor != nil {
		if !interceptor(caller, property) {
			msg := fmt.Sprintf("illegal access violation: %s cannot access %s::%s", caller.Name(), origin.Name(), property)
			panic(errors.New(msg))
		}
	}
	log.Debugf("SecurityProxy: SecurityInterceptor check success: %s", property)
}
