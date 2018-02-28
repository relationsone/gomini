package sbgoja

import (
	"github.com/relationsone/gomini"
	"github.com/dop251/goja"
	"bytes"
)

type _stackFrame struct {
	original goja.StackFrame
}

func (s _stackFrame) Position() gomini.Position {
	position := s.original.Position()
	return gomini.Position{
		Line: position.Line,
		Col:  position.Col,
	}
}

func (s _stackFrame) SrcName() string {
	return s.original.SrcName()
}

func (s _stackFrame) FuncName() string {
	return s.original.FuncName()
}

func (s _stackFrame) String() string {
	buffer := bytes.Buffer{}
	s.original.Write(&buffer)
	return buffer.String()
}
