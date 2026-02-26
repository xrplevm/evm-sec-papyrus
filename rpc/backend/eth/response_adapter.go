package eth

import (
	legacytypes "github.com/cosmos/evm/rpc/types/legacy"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// AdaptTxResponse converts a legacy Ethermint MsgEthereumTxResponse to the
// cosmos.evm.vm.v1 format. Fields absent in the legacy type default to zero.
func AdaptTxResponse(legacy *legacytypes.MsgEthereumTxResponse) *evmtypes.MsgEthereumTxResponse {
	logs := make([]*evmtypes.Log, len(legacy.Logs))
	for i, l := range legacy.Logs {
		logs[i] = &evmtypes.Log{
			Address:     l.Address,
			Topics:      l.Topics,
			Data:        l.Data,
			BlockNumber: l.BlockNumber,
			TxHash:      l.TxHash,
			TxIndex:     l.TxIndex,
			BlockHash:   l.BlockHash,
			Index:       l.Index,
			Removed:     l.Removed,
			// BlockTimestamp (field 10) absent in legacy → zero
		}
	}
	return &evmtypes.MsgEthereumTxResponse{
		Hash:    legacy.Hash,
		Logs:    logs,
		Ret:     legacy.Ret,
		VmError: legacy.VmError,
		GasUsed: legacy.GasUsed,
		// MaxUsedGas, BlockHash, BlockTimestamp absent in legacy → zero
	}
}
