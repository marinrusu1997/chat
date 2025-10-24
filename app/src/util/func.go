package util

import (
	"runtime"
)

func GetFunctionName() string {
	// pc: program counter, file: file name, line: line number, ok: success bool
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return "unknown function"
	}

	// Get the function object
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown function"
	}

	// fn.Name() returns the fully qualified name, e.g., "main.MyStruct.MyMethod"
	return fn.Name()
}

func DereferenceString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
