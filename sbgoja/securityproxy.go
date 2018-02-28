package sbgoja

import (
	"github.com/dop251/goja"
	"github.com/relationsone/gomini"
)

type securityProxy struct {
	sandbox *sandbox
}

func newSecurityProxy(sandbox *sandbox) *securityProxy {
	return &securityProxy{
		sandbox: sandbox,
	}
}

func (s *securityProxy) makeProxy(target *goja.Object, propertyName string, origin, caller gomini.Bundle) (*goja.Object, error) {
	originRuntime := unwrapGojaRuntime(origin)
	callerRuntime := unwrapGojaRuntime(caller)

	handler := &goja.ProxyTrapConfig{
		GetPrototypeOf: func(target *goja.Object) *goja.Object {
			return callerRuntime.NewTypeError("Proxies have no prototypes")
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
				Getter: callerRuntime.ToValue(func() goja.Value {
					// TODO
					return nil
				}),
			}
		},
		Get: func(target *goja.Object, property string, receiver *goja.Object) goja.Value {
			s.accessCheck(propertyName+"."+property+".get", origin, caller)

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
			s.accessCheck(propertyName+"."+property+".has", origin, caller)

			return target.Get(property) != nil
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			return callerRuntime.ToValue(target.Keys()).(*goja.Object)
		},
		Apply: func(target *goja.Object, this *goja.Object, argumentsList []goja.Value) goja.Value {
			s.accessCheck(propertyName+".apply", origin, caller)

			thisProxy, err := s.makeProxy(this, propertyName+".this", caller, origin)
			if err != nil {
				panic(err)
			}

			var function func(goja.FunctionCall) goja.Value
			err = originRuntime.ExportTo(target, &function)
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
			err := originRuntime.ExportTo(target, &constructor)
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

			return proxy
		},
	}

	proxy := callerRuntime.NewProxy(target, handler, false, false)
	return callerRuntime.ToValue(proxy).(*goja.Object), nil
}

func (s *securityProxy) accessCheck(propertyName string, origin, caller gomini.Bundle) {
	if err := sandboxSecurityCheck(propertyName, origin, caller); err != nil {
		panic(err)
	}
}
