package host

import "github.com/bytecodealliance/wasmtime-go"

type HostFunctionCallBack = func(*wasmtime.Caller, []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap)

type HostFunction interface {
	// Name of the method as describe in WASM
	Name() string

	// FuncType represents the function signature
	FuncType() *wasmtime.FuncType

	// Callback refers to the implementation of function logic on the host
	Callback() HostFunctionCallBack

	// Initialize inits with necessary Wasmtime elements such as allocation, deallocation functions and memory
	Initialize(allocFunc, deallocFunc *wasmtime.Func, memory *wasmtime.Memory, nodeAddress string, quorumType int)
}
