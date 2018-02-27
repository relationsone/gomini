package kmodules

import (
	"github.com/relationsone/gomini"
	"github.com/apex/log"
	"fmt"
)

const kmoduleLoggerId = "3c6bddf9-7c84-41c4-8796-22379c4a5e29"

type kmoduleLogger struct {
}

func NewLoggerModule() gomini.KernelModule {
	return &kmoduleLogger{}
}

func (*kmoduleLogger) ID() string {
	return kmoduleLoggerId
}

func (*kmoduleLogger) Name() string {
	return "logger"
}

func (*kmoduleLogger) ApiDefinitionFile() string {
	return "/kernel/@types/logger"
}

func (*kmoduleLogger) SecurityInterceptor() gomini.SecurityInterceptor {
	return func(caller gomini.Bundle, property string) bool {
		// Everyone's supposed to use the logger API
		return true
	}
}

func (*kmoduleLogger) KernelModuleBinder() gomini.KernelModuleBinder {
	return func(bundle gomini.Bundle, builder gomini.JsObjectBuilder) {
		builder.
			DefineObjectProperty("log", func(builder gomini.JsObjectBuilder) {
			builder.DefineFunction("info", func(call gomini.JsFunctionCall) gomini.JsValue {
				sandbox := bundle.Sandbox()

				if len(call.Arguments) < 1 {
					return bundle.NewTypeError("info called without arguments")
				}

				msg := call.Argument(0).String()
				if len(call.Arguments) > 1 {
					args := make([]interface{}, len(call.Arguments)-1)
					for i := 1; i < len(call.Arguments); i++ {
						args[i-1] = call.Argument(i)
					}
					msg = fmt.Sprintf(msg, args...)
				}

				stackFrames := sandbox.CaptureCallStack(1)
				frame := stackFrames[0]
				pos := frame.Position()
				log.Infof("%s#%s[%d:%d]: %s", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col, msg)
				return bundle.Undefined()
			})
		})
	}
}
