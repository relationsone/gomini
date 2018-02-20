package gomini

import (
	"github.com/apex/log"
	"github.com/dop251/goja"
	"strings"
)

func consoleApi() ApiProviderBinder {
	return func(kernel Bundle, bundle Bundle, builder ApiBuilder) {
		consoleBuilder := func(builder ObjectBuilder) {
			builder.DefineFunction("log", func(call goja.FunctionCall) goja.Value {
				stackFrames := bundle.Sandbox().CaptureCallStack(2)
				frame := stackFrames[0]
				if strings.HasPrefix(frame.SrcName(), "<native>") {
					if len(stackFrames) > 1 {
						frame = stackFrames[1]
					} else {
						frame = goja.StackFrame{}
					}
				}
				pos := frame.Position()
				msg := call.Argument(0)
				log.Infof("%s::%s[%d:%d]: %s", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col, msg)
				return goja.Undefined()

			}).DefineFunction("stackTrace", func() {
				stackFrames := bundle.Sandbox().CaptureCallStack(-1)
				log.Infof("Dumping CallStack:")
				for _, frame := range stackFrames {
					pos := frame.Position()
					log.Infof("\t%s::%s[%d:%d]", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col)
				}
			}).EndObject()
		}

		builder.DefineObject("console", consoleBuilder).EndApi()
	}
}

func timeoutApi() ApiProviderBinder {
	return func(kernel Bundle, bundle Bundle, builder ApiBuilder) {
		builder.DefineFunction("setTimeout", func(call goja.FunctionCall) goja.Value {
			return goja.Null()
		}).EndApi()
	}
}
