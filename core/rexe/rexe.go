package rexe

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/bytecodealliance/wasmtime-go"
)

// WasmModule encapsulates the WASM module and its associated functions.
type RexeModule struct {
	// Wasmtime Runtime elements
	engine      *wasmtime.Engine
	store       *wasmtime.Store
	instance    *wasmtime.Instance
	memory      *wasmtime.Memory
	allocFunc   *wasmtime.Func
	deallocFunc *wasmtime.Func

	// Rubix Blockchain elements
	nodeAddress string
	quorumType  int

	// Context
	// wasmCtx *wasmContext.WasmContext
}

type ContractResult struct {
	SuccessMsg string `json:"success_msg"`
	FailureMsg string `json:"failure_msg"`
}

func (r *RexeModule) NewRexe(contractHash string) (*RexeModule, error) {
	// Define Wasm Module with default params
	rexeModule := &RexeModule{
		nodeAddress: "http://localhost:20000",
		quorumType:  2,
	}
	wasmFilePath := fmt.Sprintf("%s/%s.wasm", "rubix-folder", contractHash)
	// Read the WASM file
	wasmBytes, err := os.ReadFile(wasmFilePath)
	if err != nil {
		return nil, err
	}

	rexeModule.engine = wasmtime.NewEngine()
	rexeModule.store = wasmtime.NewStore(rexeModule.engine)
	// linker := wasmtime.NewLinker(rexeModule.engine)

	// for _, hf := range registry.GetHostFunctions() {
	// 	err := linker.Define("env", hf.Name(), wasmtime.NewFunc(
	// 		wasmModule.store,
	// 		hf.FuncType(),
	// 		hf.Callback(),
	// 	))
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to define host function %s: %w", hf.Name(), err)
	// 	}
	// }

	module, err := wasmtime.NewModule(rexeModule.engine, wasmBytes)
	if err != nil {
		return nil, err
	}

	rexeModule.instance, err = wasmtime.NewInstance(rexeModule.store, module, nil)
	if err != nil {
		return nil, err
	}

	rexeModule.memory = rexeModule.instance.GetExport(rexeModule.store, "memory").Memory()
	if rexeModule.memory == nil {
		return nil, errors.New("failed to find memory export")
	}

	rexeModule.allocFunc = rexeModule.instance.GetExport(rexeModule.store, "alloc").Func()
	if rexeModule.allocFunc == nil {
		return nil, errors.New("failed to find alloc function")
	}

	rexeModule.deallocFunc = rexeModule.instance.GetExport(rexeModule.store, "dealloc").Func()
	if rexeModule.deallocFunc == nil {
		return nil, errors.New("failed to find dealloc function")
	}

	// Apply Wasm Configurations
	// for _, opt := range wasmModuleOpts {
	// 	opt(wasmModule)
	// }

	// Initialize all host functions with allocFunc, deallocFunc, and memory
	// for _, hf := range registry.GetHostFunctions() {
	// 	hf.Initialize(
	// 		wasmModule.allocFunc,
	// 		wasmModule.deallocFunc,
	// 		wasmModule.memory,
	// 		wasmModule.nodeAddress,
	// 		wasmModule.quorumType,
	// 		wasmModule.wasmCtx,
	// 	)

	// }

	return rexeModule, nil
}

// allocate allocates memory in WASM and copies the data.
func (w *RexeModule) allocate(data []byte) (int32, error) {
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
func (w *RexeModule) deallocate(ptr int32, size int32) error {
	_, err := w.deallocFunc.Call(w.store, ptr, size)
	return err
}

// CallFunctions invokes the exported WASM function and returns the
// result in string format
func (r *RexeModule) Call(args string) ContractResult {
	// Parse the JSON string
	contractResult := ContractResult{}
	var inputMap map[string]interface{}
	err := json.Unmarshal([]byte(args), &inputMap)
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to parse input JSON: %v", err)
		return contractResult
	}
	if len(inputMap) != 1 {
		contractResult.FailureMsg = "input JSON must contain exactly one function"
		return contractResult
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
		contractResult.FailureMsg = fmt.Sprintf("failed to serialize input struct: %v", err)
		return contractResult
	}

	// Allocate memory for input data
	inputPtr, err := r.allocate(inputJSON)
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to allocate memory for input data: %v", err)
		return contractResult
	}
	defer r.deallocate(inputPtr, int32(len(inputJSON)))

	// Prepare pointers for output data
	outputPtrPtr, err := r.allocate(make([]byte, 4)) // 4 bytes for pointer
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to allocate memory for output_ptr_ptr: %v", err)
		return contractResult
	}
	defer r.deallocate(outputPtrPtr, 4)

	outputLenPtr, err := r.allocate(make([]byte, 4)) // 4 bytes for length
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to allocate memory for output_len_ptr: %v", err)
		return contractResult
	}
	defer r.deallocate(outputLenPtr, 4)

	// Retrieve the wrapper function
	extern := r.instance.GetExport(r.store, wrapperFuncName)
	if extern == nil {
		contractResult.FailureMsg = fmt.Sprintf("function %s does not exist in the contract", wrapperFuncName)
		return contractResult
	}

	function := extern.Func()
	if function == nil {
		contractResult.FailureMsg = fmt.Sprintf("export %s is not a function", wrapperFuncName)
		return contractResult
	}

	// Call the wrapper function
	ret, err := function.Call(r.store, inputPtr, len(inputJSON), outputPtrPtr, outputLenPtr)
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("error calling WASM function: %v", err)
		return contractResult
	}

	// Check return code
	retCode, ok := ret.(int32)
	if !ok {
		contractResult.FailureMsg = "unexpected return type from WASM function"
		return contractResult
	}

	// Read output_ptr_ptr and output_len_ptr
	memoryData := r.memory.UnsafeData(r.store)
	if len(memoryData) < int(outputPtrPtr)+4 || len(memoryData) < int(outputLenPtr)+8 {
		contractResult.FailureMsg = "invalid memory access for output pointers"
		return contractResult
	}

	outputPtr := int32(binary.LittleEndian.Uint32(memoryData[outputPtrPtr:]))
	outputLen := int32(binary.LittleEndian.Uint64(memoryData[outputLenPtr:]))

	// Validate memory bounds
	if outputPtr < 0 || outputPtr+outputLen > int32(len(memoryData)) {
		contractResult.FailureMsg = "output data exceeds memory bounds"
		return contractResult
	}

	// Read output data
	outputData := make([]byte, outputLen)
	copy(outputData, memoryData[outputPtr:outputPtr+outputLen])

	// Deserialize output data
	var output interface{}
	err = json.Unmarshal(outputData, &output)
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to deserialize output data: %v", err)
		return contractResult
	}

	// Deallocate output data
	err = r.deallocate(outputPtr, outputLen)
	if err != nil {
		contractResult.FailureMsg = fmt.Sprintf("failed to deallocate output data: %v", err)
		return contractResult
	}

	// Type assert output to string
	contractOutputStr, ok := output.(string)
	if !ok {
		contractResult.FailureMsg = "expected output of contract to be string type"
		return contractResult
		//return "", fmt.Errorf("expected output of contract to be string type")
	}

	if retCode != 0 {
		contractResult.FailureMsg = fmt.Sprintf("contract execution failed with code %d: %s", retCode, contractOutputStr)
		return contractResult
	}

	return contractResult
}
