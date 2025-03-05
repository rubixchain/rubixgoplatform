package wasmbridge

import (
	// "github.com/rubixchain/rubixgoplatform/client"
	"github.com/rubixchain/rubixgoplatform/wasmbridge/host"
	"github.com/rubixchain/rubixgoplatform/wasmbridge/host/ft"
	"github.com/rubixchain/rubixgoplatform/wasmbridge/host/generic"
)

type MintFTData struct {
	Did        string `json:"did"`
	FtCount    int32  `json:"ft_count"`
	FtName     string `json:"ft_name"`
	TokenCount int32  `json:"token_count"`
}

// HostFunctionRegistry manages the registration of host functions.
type HostFunctionRegistry struct {
	hostFunctions []host.HostFunction
}

// NewHostFunctionRegistry creates a new registry with predefined host functions.
func NewHostFunctionRegistry(clientInstance ft.ClientInterface, ftData ft.MintFTData) *HostFunctionRegistry {
	registry := &HostFunctionRegistry{
		hostFunctions: []host.HostFunction{},
	}

	// Register predefined host functions
	registry.Register(generic.NewDoApiCall())
	registry.Register(ft.NewDoMintFTApiCall(clientInstance, ftData))
	// registry.Register(ft.NewDoTransferFTApiCall(clientInstance))

	return registry
}

// Register adds a new host function to the registry.
func (r *HostFunctionRegistry) Register(hf host.HostFunction) {
	r.hostFunctions = append(r.hostFunctions, hf)
}

// GetHostFunctions returns all registered host function names.
func (r *HostFunctionRegistry) GetHostFunctions() []host.HostFunction {
	return r.hostFunctions
}
