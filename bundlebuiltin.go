package gomini

import (
	"github.com/apex/log"
	"github.com/dop251/goja"
	"strings"
)

func consoleApi() ApiProviderBinder {
	return func(kernel Bundle, bundle Bundle, builder BundleObjectBuilder) {
		consoleBuilder := func(builder JsObjectBuilder) {
			builder.DefineFunction("log", func(call JsFunctionCall) JsValue {
				stackFrames := bundle.Sandbox().CaptureCallStack(2)
				var frame goja.StackFrame
				for i := 0; i < len(stackFrames); i++ {
					frame = stackFrames[i]
					if !strings.HasPrefix(frame.SrcName(), "<native>") {
						break
					}
				}
				if &frame == nil {
					frame = goja.StackFrame{}
				}
				pos := frame.Position()
				msg := call.Argument(0)
				log.Infof("%s::%s[%d:%d]: %s", frame.SrcName(), frame.FuncName(), pos.Line, pos.Col, msg)
				return bundle.Undefined()

			}).DefineGoFunction("stackTrace", func() {
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
	return func(kernel Bundle, bundle Bundle, builder BundleObjectBuilder) {
		builder.DefineFunction("setTimeout", func(call JsFunctionCall) JsValue {
			return bundle.Null()
		})
	}
}
