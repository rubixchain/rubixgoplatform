// Package wasmbridge implements functions to interact
// with WASM binaries
package wasmbridge

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/bytecodealliance/wasmtime-go"
	// "github.com/rubixchain/rubix-wasm/go-wasm-bridge/host"
)

type HostFunctionCallBack = func(*wasmtime.Caller, []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap)

type HostFunction interface {
	// Name of the method as describe in WASM
	Name() string

	// FuncType represents the function signature
	FuncType() *wasmtime.FuncType

	// Callback refers to the implementation of function logic on the host
	Callback() HostFunctionCallBack

	// Initialize inits with necessary Wasmtime elements such as allocation, deallocation functions and memory
	Initialize(allocFunc, deallocFunc *wasmtime.Func, memory *wasmtime.Memory)
}

// WasmModule encapsulates the WASM module and its associated functions.
type WasmModule struct {
	// Wasmtime Runtime elements
	engine      *wasmtime.Engine
	store       *wasmtime.Store
	instance    *wasmtime.Instance
	memory      *wasmtime.Memory
	allocFunc   *wasmtime.Func
	deallocFunc *wasmtime.Func

	// Rubix Blockchain elements
	// nodeAddress string
	// quorumType  int
}

type SmartContractDataReply struct {
	BasicResponse
	SCTDataReply []SCTDataReply
}

type BasicResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

type SCTDataReply struct {
	BlockNo           uint64
	BlockId           string
	SmartContractData string
}

// WasmModuleOption allows us to configure WasmModule
type WasmModuleOption func(*WasmModule)

// HostFunctionRegistry manages the registration of host functions.
// type HostFunctionRegistry struct {
// 	hostFunctions []HostFunction
// }

// Register adds a new host function to the registry.
// func (r *HostFunctionRegistry) Register(hf HostFunction) {
// 	r.hostFunctions = append(r.hostFunctions, hf)
// }

// // GetHostFunctions returns all registered host function names.
// func (r *HostFunctionRegistry) GetHostFunctions() []HostFunction {
// 	return r.hostFunctions
// }

// NewWasmModule initializes and returns a new WasmModule.

func NewWasmModule(wasmFilePath string, registry *HostFunctionRegistry, wasmModuleOpts ...WasmModuleOption) (*WasmModule, error) {
	// Define Wasm Module with default params
	wasmModule := &WasmModule{}

	// Read the WASM file
	wasmBytes, err := os.ReadFile(wasmFilePath)
	if err != nil {
		return nil, err
	}

	wasmModule.engine = wasmtime.NewEngine()
	wasmModule.store = wasmtime.NewStore(wasmModule.engine)
	linker := wasmtime.NewLinker(wasmModule.engine)

	for _, hf := range registry.GetHostFunctions() {
		err := linker.Define("env", hf.Name(), wasmtime.NewFunc(
			wasmModule.store,
			hf.FuncType(),
			hf.Callback(),
		))
		if err != nil {
			return nil, fmt.Errorf("failed to define host function %s: %w", hf.Name(), err)
		}
	}

	module, err := wasmtime.NewModule(wasmModule.engine, wasmBytes)
	if err != nil {
		return nil, err
	}

	wasmModule.instance, err = linker.Instantiate(wasmModule.store, module)
	if err != nil {
		return nil, err
	}

	wasmModule.memory = wasmModule.instance.GetExport(wasmModule.store, "memory").Memory()
	if wasmModule.memory == nil {
		return nil, errors.New("failed to find memory export")
	}

	wasmModule.allocFunc = wasmModule.instance.GetExport(wasmModule.store, "alloc").Func()
	if wasmModule.allocFunc == nil {
		return nil, errors.New("failed to find alloc function")
	}

	wasmModule.deallocFunc = wasmModule.instance.GetExport(wasmModule.store, "dealloc").Func()
	if wasmModule.deallocFunc == nil {
		return nil, errors.New("failed to find dealloc function")
	}

	// Apply Wasm Configurations
	for _, opt := range wasmModuleOpts {
		opt(wasmModule)
	}

	// Initialize all host functions with allocFunc, deallocFunc, and memory
	for _, hf := range registry.GetHostFunctions() {
		hf.Initialize(
			wasmModule.allocFunc,
			wasmModule.deallocFunc,
			wasmModule.memory,
			// wasmModule.nodeAddress,
			// wasmModule.quorumType,
		)
	}

	return wasmModule, nil
}

// func WithRubixNodeAddress(nodeAddress string) WasmModuleOption {
// 	return func(w *WasmModule) {
// 		w.nodeAddress = nodeAddress
// 	}
// }

// func WithQuorumType(quorumType int) WasmModuleOption {
// 	return func(w *WasmModule) {
// 		w.quorumType = quorumType
// 	}
// }

// func (w *WasmModule) GetNodeAddress() string {
// 	return w.nodeAddress
// }

// func (w *WasmModule) GetQuorumType() int {
// 	return w.quorumType
// }

// allocate allocates memory in WASM and copies the data.
func (w *WasmModule) allocate(data []byte) (int32, error) {
	size := len(data)
	result, err := w.allocFunc.Call(w.store, size)
	if err != nil {
		return 0, err
	}
	ptr := result.(int32)
	memoryData := w.memory.UnsafeData(w.store)
	copy(memoryData[ptr:ptr+int32(size)], data)
	return ptr, nil
}

// deallocate frees memory in WASM.
func (w *WasmModule) deallocate(ptr int32, size int32) error {
	_, err := w.deallocFunc.Call(w.store, ptr, size)
	return err
}

// CallFunctions invokes the exported WASM function and returns the
// result in string format
func (w *WasmModule) CallFunction(args string) (string, error) {
	// Parse the JSON string
	var inputMap map[string]interface{}
	err := json.Unmarshal([]byte(args), &inputMap)
	if err != nil {
		return "", fmt.Errorf("failed to parse input JSON: %v", err)
	}
	if len(inputMap) != 1 {
		return "", errors.New("input JSON must contain exactly one function")
	}

	// Extract function name and input struct
	var funcName string
	var inputStruct interface{}
	for key, value := range inputMap {
		funcName = key
		inputStruct = value
	}

	// Append '' suffix to get the actual function name which is wrapped by Rust libs
	wrapperFuncName := funcName + "_"

	// Serialize the input struct to JSON
	inputJSON, err := json.Marshal(inputStruct)
	if err != nil {
		return "", fmt.Errorf("failed to serialize input struct: %v", err)
	}

	// Allocate memory for input data
	inputPtr, err := w.allocate(inputJSON)
	if err != nil {
		return "", fmt.Errorf("failed to allocate memory for input data: %v", err)
	}
	defer w.deallocate(inputPtr, int32(len(inputJSON)))

	// Prepare pointers for output data
	outputPtrPtr, err := w.allocate(make([]byte, 4)) // 4 bytes for pointer
	if err != nil {
		return "", fmt.Errorf("failed to allocate memory for output_ptr_ptr: %v", err)
	}
	defer w.deallocate(outputPtrPtr, 4)

	outputLenPtr, err := w.allocate(make([]byte, 4)) // 4 bytes for length
	if err != nil {
		return "", fmt.Errorf("failed to allocate memory for output_len_ptr: %v", err)
	}
	defer w.deallocate(outputLenPtr, 4)

	// Retrieve the wrapper function
	extern := w.instance.GetExport(w.store, wrapperFuncName)
	if extern == nil {
		return "", fmt.Errorf("function %s does not exist in the contract", funcName)
	}

	function := extern.Func()
	if function == nil {
		return "", fmt.Errorf("export %s is not a function", funcName)
	}

	// Call the wrapper function
	ret, err := function.Call(w.store, inputPtr, len(inputJSON), outputPtrPtr, outputLenPtr)
	if err != nil {
		return "", fmt.Errorf("error calling WASM function: %v", err)
	}

	// Check return code
	retCode, ok := ret.(int32)
	if !ok {
		return "", errors.New("unexpected return type from WASM function")
	}

	// Read output_ptr_ptr and output_len_ptr
	memoryData := w.memory.UnsafeData(w.store)
	if len(memoryData) < int(outputPtrPtr)+4 || len(memoryData) < int(outputLenPtr)+8 {
		return "", errors.New("invalid memory access for output pointers")
	}

	outputPtr := int32(binary.LittleEndian.Uint32(memoryData[outputPtrPtr:]))
	outputLen := int32(binary.LittleEndian.Uint64(memoryData[outputLenPtr:]))

	// Validate memory bounds
	if outputPtr < 0 || outputPtr+outputLen > int32(len(memoryData)) {
		return "", errors.New("output data exceeds memory bounds")
	}

	// Read output data
	outputData := make([]byte, outputLen)
	copy(outputData, memoryData[outputPtr:outputPtr+outputLen])

	// Deserialize output data
	var output interface{}
	err = json.Unmarshal(outputData, &output)
	if err != nil {
		return "", fmt.Errorf("failed to deserialize output data: %v", err)
	}

	// Deallocate output data
	err = w.deallocate(outputPtr, outputLen)
	if err != nil {
		return "", fmt.Errorf("failed to deallocate output data: %v", err)
	}

	// Type assert output to string
	contractOutputStr, ok := output.(string)
	if !ok {
		return "", fmt.Errorf("expected output of contract to be string type")
	}

	if retCode != 0 {
		return "", fmt.Errorf("contract execution failed: %v", contractOutputStr)
	}

	return contractOutputStr, nil
}
