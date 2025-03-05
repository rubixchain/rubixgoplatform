package generic

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/rubixchain/rubixgoplatform/wasmbridge/host"
	utils "github.com/rubixchain/rubixgoplatform/wasmbridge/wasmutil"
)

type DoApiCall struct {
	allocFunc *wasmtime.Func
	memory    *wasmtime.Memory
}

func NewDoApiCall() *DoApiCall {
	return &DoApiCall{}
}

func (h *DoApiCall) Name() string {
	return "do_api_call"
}

func (h *DoApiCall) FuncType() *wasmtime.FuncType {
	return wasmtime.NewFuncType(
		[]*wasmtime.ValType{
			wasmtime.NewValType(wasmtime.KindI32), // url_ptr
			wasmtime.NewValType(wasmtime.KindI32), // url_len
			wasmtime.NewValType(wasmtime.KindI32), // resp_ptr_ptr
			wasmtime.NewValType(wasmtime.KindI32), // resp_len_ptr
		},
		[]*wasmtime.ValType{wasmtime.NewValType(wasmtime.KindI32)}, // return i32
	)
}

func (h *DoApiCall) Initialize(allocFunc, deallocFunc *wasmtime.Func, memory *wasmtime.Memory, nodeAddress string, quorumType int) {
	h.allocFunc = allocFunc
	h.memory = memory
}

func (h *DoApiCall) Callback() host.HostFunctionCallBack {
	return h.callback
}

func (h *DoApiCall) callback(
	caller *wasmtime.Caller,
	args []wasmtime.Val,
) ([]wasmtime.Val, *wasmtime.Trap) {
	// Validate the number of arguments
	inputArgs, outputArgs := utils.HostFunctionParamExtraction(args, true, true)

	// Extract URL bytes and convert to string
	urlBytes, memory, err := utils.ExtractDataFromWASM(caller, inputArgs)
	if err != nil {
		fmt.Println("Failed to extract data from WASM", err)
		return utils.HandleError(err.Error())
	}
	h.memory = memory // Assign memory to Host struct for future use
	url := string(urlBytes)

	// Make HTTP GET request to the provided URL
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("HTTP request failed: %v\n", err)
		return utils.HandleError(err.Error())
	}
	defer resp.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response body: %v\n", err)
		return utils.HandleError(err.Error())
	}

	responseStr := string(body)
	err = utils.UpdateDataToWASM(caller, h.allocFunc, responseStr, outputArgs)
	if err != nil {
		fmt.Println("Failed to update data to WASM", err)
		return utils.HandleError(err.Error())
	}

	return utils.HandleOk() // Success
}
