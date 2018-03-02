package gomini

import (
	"reflect"
)

type Sandbox interface {
	NewObject() Object

	// NewObjectCreator returns an ObjectCreator instance which can be used
	// to define script Object instances.
	//
	// The provided objectName will be used to register the object with the
	// given parent when ObjectCreator::BuildInto is called.
	NewObjectCreator(objectName string) ObjectCreator
	NewNamedNativeFunction(functionName string, function GoFunction) Value
	NewTypeError(args ...interface{}) Object
	NewError(err error) Object

	NewModuleProxy(object Object, objectName string, caller Bundle) (Object, error)
	IsAccessible(module Module, caller Bundle) error

	Compile(filename, source string) (script Script, cacheable bool, err error)
	Execute(script Script) (Value, error)
	CaptureCallStack(maxStackFrames int) []StackFrame
	NewDebugger() (interface{}, error)

	Global() Object
	NullValue() Value
	UndefinedValue() Value

	ToValue(value interface{}) Value
	Export(value Value, target interface{}) error
}

type StackFrame interface {
	Position() Position
	SrcName() string
	FuncName() string
	String() string
}

type Position struct {
	Line, Col int
}

type Script interface {
}

type FunctionCall struct {
	This      Value
	Arguments []Value
	Bundle    Bundle
}

func (f FunctionCall) Argument(idx int) Value {
	return f.Arguments[idx]
}

type Getter func() (value interface{})
type Setter func(value interface{})

type NativeFunction func(call FunctionCall) Value
type GoFunction interface{}
type Callable func(this Value, arguments ...Value) (Value, error)

type ObjectBinder func(builder ObjectBuilder)

type PropertyFlag uint8

const (
	Flag_NotSet PropertyFlag = iota
	Flag_False
	Flag_True
)

type PropertyDescriptor struct {
	Original interface{}

	Value Value

	Getter Getter
	Setter Setter

	Writable     PropertyFlag
	Configurable PropertyFlag
	Enumerable   PropertyFlag
}

type Value interface {
	ToInteger() int64
	ToFloat() float64
	ToBoolean() bool
	ToNumber() Value
	ToString() Value

	ToObject() Object

	SameAs(other Value) bool
	Equals(other Value) bool
	StrictEquals(other Value) bool

	Export() interface{}
	ExportType() reflect.Type

	String() string

	IsObject() bool
	IsArray() bool
	IsDefined() bool

	Unwrap() interface{}
}

type Object interface {
	Value

	Get(name string) Value
	PropertyDescriptor(name string) PropertyDescriptor

	Freeze() Object
	DeepFreeze() Object

	DefineFunction(functionName, propertyName string, function NativeFunction) Object
	DefineGoFunction(functionName, propertyName string, function GoFunction) Object
	DefineConstant(constantName string, value interface{}) Object
	DefineSimpleProperty(propertyName string, value interface{}) Object
	DefineObjectProperty(objectName string, objectBinder ObjectBinder) Object
	DefineAccessorProperty(propertyName string, getter Getter, setter Setter) Object
}

type ObjectBuilder interface {
	DefineFunction(functionName, propertyName string, function NativeFunction) ObjectBuilder
	DefineGoFunction(functionName, propertyName string, function GoFunction) ObjectBuilder
	DefineConstant(constantName string, value interface{}) ObjectBuilder
	DefineSimpleProperty(propertyName string, value interface{}) ObjectBuilder
	DefineObjectProperty(objectName string, objectBinder ObjectBinder) ObjectBuilder
	DefineAccessorProperty(propertyName string, getter Getter, setter Setter) ObjectBuilder
}

type ObjectCreator interface {
	ObjectBuilder
	Build() Object
	BuildInto(objectName string, parent Object)
}
