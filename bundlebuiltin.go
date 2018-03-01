package gomini

import (
	"github.com/apex/log"
	"strings"
)

func consoleApi() ApiProviderBinder {
	return func(kernel Bundle, bundle Bundle, builder ObjectCreator) {
		consoleBuilder := func(builder ObjectBuilder) {
			builder.DefineFunction("log", "log", func(call FunctionCall) Value {
				stackFrames := bundle.Sandbox().CaptureCallStack(2)
				var frame StackFrame
				for i := 0; i < len(stackFrames); i++ {
					frame = stackFrames[i]
					if !strings.HasPrefix(frame.SrcName(), "<native>") {
						break
					}
				}
				if &frame == nil {
					frame = emptyStackFrame
				}
				pos := frame.Position()
				msg := call.Argument(0)
				log.Infof("%s::%s[%d:%d]: %s", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col, msg)
				return bundle.Undefined()

			}).DefineGoFunction("stackTrace", "stackTrace", func() {
				stackFrames := bundle.Sandbox().CaptureCallStack(-1)
				log.Infof("Dumping CallStack:")
				for _, frame := range stackFrames {
					pos := frame.Position()
					log.Infof("\t%s::%s[%d:%d]", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col)
				}
			})
		}

		builder.DefineObjectProperty("console", consoleBuilder)
	}
}

func timeoutApi() ApiProviderBinder {
	return func(kernel Bundle, bundle Bundle, builder ObjectCreator) {
		builder.DefineFunction("setTimeout", "setTimeout", func(call FunctionCall) Value {
			return bundle.Null()
		})
	}
}
