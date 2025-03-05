package ft

import (
	"encoding/json"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wasmbridge/host"
	utils "github.com/rubixchain/rubixgoplatform/wasmbridge/wasmutil"
)

// ClientInterface defines the methods required by a client
type ClientInterface interface {
	CreateFT(did string, ftName string, ftCount int, tokenCount int) (*model.BasicResponse, error)
}

// CreateFTFunc calls the CreateFT method from the client
func CreateFTFunc(c ClientInterface, did string, ftName string, ftCount int, tokenCount int) (*model.BasicResponse, error) {
	return c.CreateFT(did, ftName, ftCount, tokenCount)
}

// CallFunctionInsideFT calls a function inside the FT module
func CallFunctionInsideFT(c ClientInterface, ftData MintFTData) string {
	CreateFTFunc(c, ftData.Did, ftData.FtName, int(ftData.FtCount), int(ftData.TokenCount))
	return "FT Function called"
}

// DoMintFTApiCall struct holds necessary fields for execution
type DoMintFTApiCall struct {
	allocFunc    *wasmtime.Func
	memory       *wasmtime.Memory
	client       ClientInterface                                                                          // ✅ Store client instance
	callbackfunc func(*wasmtime.Caller, []wasmtime.Val, ClientInterface) ([]wasmtime.Val, *wasmtime.Trap) // ✅ Store callback function
}

// MintFTData represents the JSON structure for minting FT
type MintFTData struct {
	Did        string `json:"did"`
	FtCount    int32  `json:"ft_count"`
	FtName     string `json:"ft_name"`
	TokenCount int32  `json:"token_count"`
}

// NewDoMintFTApiCall creates a new instance of DoMintFTApiCall
func NewDoMintFTApiCall(client ClientInterface, ftData MintFTData) *DoMintFTApiCall {
	return &DoMintFTApiCall{
		client: client, // ✅ Inject client
		callbackfunc: func(caller *wasmtime.Caller, args []wasmtime.Val, c ClientInterface) ([]wasmtime.Val, *wasmtime.Trap) {
			// ✅ Use injected client to call function
			response := CallFunctionInsideFT(c, ftData)
			fmt.Println("The response received:", response)
			return utils.HandleOk()
		},
	}
}

// Name returns the function name
func (h *DoMintFTApiCall) Name() string {
	return "do_mint_ft"
}

// FuncType returns the Wasm function signature
func (h *DoMintFTApiCall) FuncType() *wasmtime.FuncType {
	return wasmtime.NewFuncType(
		[]*wasmtime.ValType{
			wasmtime.NewValType(wasmtime.KindI32), // input_ptr
			wasmtime.NewValType(wasmtime.KindI32), // input_len
			wasmtime.NewValType(wasmtime.KindI32), // resp_ptr_ptr
			wasmtime.NewValType(wasmtime.KindI32), // resp_len_ptr
		},
		[]*wasmtime.ValType{wasmtime.NewValType(wasmtime.KindI32)}, // return i32
	)
}

// Initialize sets up the function and memory references
func (h *DoMintFTApiCall) Initialize(allocFunc, deallocFunc *wasmtime.Func, memory *wasmtime.Memory) {
	h.allocFunc = allocFunc
	h.memory = memory
}

// Callback returns the function callback for execution
func (h *DoMintFTApiCall) Callback() host.HostFunctionCallBack {
	return func(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
		return h.callback(caller, args, h.client) // ✅ Use injected client
	}
}

// callback function for executing the mint FT operation
func (h *DoMintFTApiCall) callback(
	caller *wasmtime.Caller,
	args []wasmtime.Val,
	c ClientInterface) ([]wasmtime.Val, *wasmtime.Trap) {

	inputArgs, outputArgs := utils.HostFunctionParamExtraction(args, true, true)

	inputBytes, memory, err := utils.ExtractDataFromWASM(caller, inputArgs)
	if err != nil {
		fmt.Println("Failed to extract data from WASM", err)
		return utils.HandleError(err.Error())
	}
	h.memory = memory

	var mintFTData MintFTData
	err3 := json.Unmarshal(inputBytes, &mintFTData)
	if err3 != nil {
		fmt.Println("Error unmarshaling mintftdata:", err3)
		return utils.HandleError(err3.Error())
	}

	// ✅ Use h.client instead of passing it around
	response := CallFunctionInsideFT(h.client, mintFTData)
	fmt.Println("The response received:", response)

	err = utils.UpdateDataToWASM(caller, h.allocFunc, response, outputArgs)
	if err != nil {
		fmt.Println("Failed to update data to WASM", err)
		return utils.HandleError(err.Error())
	}

	return utils.HandleOk()
}
