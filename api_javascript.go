package gomini

import (
	"reflect"
	"github.com/dop251/goja"
)

type JsFunctionCall struct {
	This      JsValue
	Arguments []JsValue
	Bundle    Bundle
}

func (f JsFunctionCall) Argument(idx int) JsValue {
	return f.Arguments[idx]
}

type JsGetter func() (value Any)
type JsSetter func(value Any)

type JsNativeFunction func(call JsFunctionCall) JsValue
type JsGoFunction interface{}
type JsCallable func(this JsValue, arguments ...JsValue) (JsValue, error)

type JsPropertyFlag uint8

type JsObjectBinder func(builder JsObjectBuilder)

const (
	Flag_NotSet JsPropertyFlag = iota
	Flag_False
	Flag_True
)

type JsPropertyDescriptor struct {
	original goja.PropertyDescriptor

	Value JsValue

	Getter JsGetter
	Setter JsSetter

	Writable     JsPropertyFlag
	Configurable JsPropertyFlag
	Enumerable   JsPropertyFlag
}

func (pd JsPropertyDescriptor) unwrap() goja.PropertyDescriptor {
	return pd.original
}

type JsValue interface {
	ToInteger() int64
	ToFloat() float64
	ToBoolean() bool
	ToNumber() JsValue
	ToString() JsValue

	ToObject() JsObject

	SameAs(other JsValue) bool
	Equals(other JsValue) bool
	StrictEquals(other JsValue) bool

	Export() interface{}
	ExportType() reflect.Type

	String() string

	unwrap() goja.Value
}

type JsObject interface {
	JsValue

	PropertyDescriptor(name string) JsPropertyDescriptor

	Freeze() JsObject

	DefineFunction(functionName string, function JsNativeFunction) JsObject
	DefineGoFunction(functionName string, function JsGoFunction) JsObject
	DefineConstant(constantName string, value Any) JsObject
	DefineSimpleProperty(propertyName string, value Any) JsObject
	DefineObjectProperty(objectName string, objectBinder JsObjectBinder) JsObject
	DefineAccessorProperty(propertyName string, getter JsGetter, setter JsSetter) JsObject
}

type JsObjectBuilder interface {
	DefineFunction(functionName string, function JsNativeFunction) JsObjectBuilder
	DefineGoFunction(functionName string, function JsGoFunction) JsObjectBuilder
	DefineConstant(constantName string, value Any) JsObjectBuilder
	DefineSimpleProperty(propertyName string, value Any) JsObjectBuilder
	DefineObjectProperty(objectName string, objectBinder JsObjectBinder) JsObjectBuilder
	DefineAccessorProperty(propertyName string, getter JsGetter, setter JsSetter) JsObjectBuilder
}

type jsFunctionDefinition struct {
	functionName string
	function     Any
}

type jsPropertyDefinition struct {
	propertyName string
	value        Any
	getter       JsGetter
	setter       JsSetter
}

type jsConstantDefinition struct {
	constantName string
	value        Any
}

type jsObjectDefinition struct {
	objects    []*jsSubObjectDefinition
	functions  []*jsFunctionDefinition
	properties []*jsPropertyDefinition
	constants  []*jsConstantDefinition
}

type jsSubObjectDefinition struct {
	objectName   string
	objectBinder JsObjectBinder
}

type JsObjectCreator struct {
	JsObjectBuilder

	bundle     Bundle
	objectName string
	assignee   *goja.Object
	*jsObjectDefinition
}
