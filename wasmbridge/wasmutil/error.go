package utils

import "github.com/bytecodealliance/wasmtime-go"

// Error handle functions

func HandleError(errMsg string) ([]wasmtime.Val, *wasmtime.Trap) {
	return []wasmtime.Val{wasmtime.ValI32(1)}, wasmtime.NewTrap(errMsg)
}

func HandleOk() ([]wasmtime.Val, *wasmtime.Trap) {
	return []wasmtime.Val{wasmtime.ValI32(0)}, nil
}
