package gomini

import (
	"github.com/dop251/goja"
)

type securityProxy struct {
	vm *goja.Runtime
}

func newSecurityProxy(bundle Bundle) (*securityProxy, error) {
	return &securityProxy{
		vm: bundle.Sandbox(),
	}, nil
}

func (s *securityProxy) makeProxy(target *goja.Object, propertyName string, origin, caller Bundle) (goja.Value, error) {
	handler := &goja.ProxyTrapConfig{
		GetPrototypeOf: func(target *goja.Object) *goja.Object {
			return caller.Sandbox().NewTypeError("Proxies have no prototypes")
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
			sandboxSecurityCheck(propertyName+"."+property+".get", origin, caller)

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
			sandboxSecurityCheck(propertyName+"."+property+".has", origin, caller)

			return target.Get(property) != nil
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			return caller.ToValue(target.Keys()).(*goja.Object)
		},
		Apply: func(target *goja.Object, this *goja.Object, argumentsList []goja.Value) goja.Value {
			sandboxSecurityCheck(propertyName+".apply", origin, caller)

			thisProxy, err := s.makeProxy(this, propertyName+".this", caller, origin)
			if err != nil {
				panic(err)
			}

			var function func(goja.FunctionCall) goja.Value
			err = origin.Sandbox().ExportTo(target, &function)
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
			err := origin.Sandbox().ExportTo(target, &constructor)
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

	proxy := caller.Sandbox().NewProxy(target, handler, false, false)
	return caller.ToValue(proxy), nil
}
