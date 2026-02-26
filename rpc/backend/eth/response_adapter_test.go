package eth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	legacytypes "github.com/cosmos/evm/rpc/types/legacy"
	"github.com/cosmos/evm/rpc/backend/eth"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

func TestAdaptTxResponse(t *testing.T) {
	testCases := []struct {
		name   string
		legacy *legacytypes.MsgEthereumTxResponse
		check  func(t *testing.T, got *evmtypes.MsgEthereumTxResponse)
	}{
		{
			name: "all scalar fields are copied",
			legacy: &legacytypes.MsgEthereumTxResponse{
				Hash:    "0xabc",
				Ret:     []byte{0x01, 0x02},
				VmError: "execution reverted",
				GasUsed: 21000,
			},
			check: func(t *testing.T, got *evmtypes.MsgEthereumTxResponse) {
				require.Equal(t, "0xabc", got.Hash)
				require.Equal(t, []byte{0x01, 0x02}, got.Ret)
				require.Equal(t, "execution reverted", got.VmError)
				require.Equal(t, uint64(21000), got.GasUsed)
			},
		},
		{
			name: "fields absent in legacy type default to zero",
			legacy: &legacytypes.MsgEthereumTxResponse{
				Hash: "0xdef",
			},
			check: func(t *testing.T, got *evmtypes.MsgEthereumTxResponse) {
				require.Equal(t, uint64(0), got.MaxUsedGas)
				require.Nil(t, got.BlockHash)
				require.Equal(t, uint64(0), got.BlockTimestamp)
			},
		},
		{
			name:   "nil Logs field produces empty slice",
			legacy: &legacytypes.MsgEthereumTxResponse{Logs: nil},
			check: func(t *testing.T, got *evmtypes.MsgEthereumTxResponse) {
				require.Empty(t, got.Logs)
			},
		},
		{
			name: "log fields are mapped correctly",
			legacy: &legacytypes.MsgEthereumTxResponse{
				Hash: "0x111",
				Logs: []*legacytypes.Log{
					{
						Address:     "0xdeadbeef",
						Topics:      []string{"0xtopic1", "0xtopic2"},
						Data:        []byte{0xca, 0xfe},
						BlockNumber: 99,
						TxHash:      "0xabcdef",
						TxIndex:     3,
						BlockHash:   "0xblockhash",
						Index:       7,
						Removed:     true,
					},
				},
			},
			check: func(t *testing.T, got *evmtypes.MsgEthereumTxResponse) {
				require.Len(t, got.Logs, 1)
				l := got.Logs[0]
				require.Equal(t, "0xdeadbeef", l.Address)
				require.Equal(t, []string{"0xtopic1", "0xtopic2"}, l.Topics)
				require.Equal(t, []byte{0xca, 0xfe}, l.Data)
				require.Equal(t, uint64(99), l.BlockNumber)
				require.Equal(t, "0xabcdef", l.TxHash)
				require.Equal(t, uint64(3), l.TxIndex)
				require.Equal(t, "0xblockhash", l.BlockHash)
				require.Equal(t, uint64(7), l.Index)
				require.True(t, l.Removed)
				// BlockTimestamp absent in legacy → zero
				require.Equal(t, uint64(0), l.BlockTimestamp)
			},
		},
		{
			name: "multiple logs are all adapted",
			legacy: &legacytypes.MsgEthereumTxResponse{
				Logs: []*legacytypes.Log{
					{Address: "0x1", Index: 0},
					{Address: "0x2", Index: 1},
					{Address: "0x3", Index: 2},
				},
			},
			check: func(t *testing.T, got *evmtypes.MsgEthereumTxResponse) {
				require.Len(t, got.Logs, 3)
				require.Equal(t, "0x1", got.Logs[0].Address)
				require.Equal(t, "0x2", got.Logs[1].Address)
				require.Equal(t, "0x3", got.Logs[2].Address)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := eth.AdaptTxResponse(tc.legacy)
			require.NotNil(t, got)
			tc.check(t, got)
		})
	}
}
