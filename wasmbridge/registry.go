package wasmbridge

import (
	"github.com/rubixchain/rubix-wasm/go-wasm-bridge/host"
	"github.com/rubixchain/rubix-wasm/go-wasm-bridge/host/ft"
	"github.com/rubixchain/rubix-wasm/go-wasm-bridge/host/generic"
	"github.com/rubixchain/rubix-wasm/go-wasm-bridge/host/nft"
)

// HostFunctionRegistry manages the registration of host functions.
type HostFunctionRegistry struct {
	hostFunctions []host.HostFunction
}

// NewHostFunctionRegistry creates a new registry with predefined host functions.
func NewHostFunctionRegistry() *HostFunctionRegistry {
	registry := &HostFunctionRegistry{
		hostFunctions: []host.HostFunction{},
	}

	// Register predefined host functions
	registry.Register(generic.NewDoApiCall())
	registry.Register(nft.NewDoMintNFTApiCall())
	registry.Register(nft.NewDoTransferNFTApiCall())
	registry.Register(ft.NewDoMintFTApiCall())
	registry.Register(ft.NewDoTransferFTApiCall())

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
