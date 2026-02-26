package backend

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/cosmos/evm/rpc/backend/mocks"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	sdkmath "cosmossdk.io/math"
)

func TestBaseFee(t *testing.T) {
	const rawBaseFee = "500000000000000000" // 0.5 × 1e18

	expBigInt := func(s string) *big.Int {
		n, _ := new(big.Int).SetString(s, 10)
		return n
	}

	testCases := []struct {
		name          string
		blockRes      *tmrpctypes.ResultBlockResults
		mockSetup     func(*mocks.EVMQueryClient)
		expBaseFee    *big.Int
		expErrContain string // non-empty → expect error containing this substring
	}{
		{
			name: "gRPC success — returns base fee from query",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 1,
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(&evmtypes.QueryBaseFeeResponse{
						BaseFee: func() *sdkmath.Int { v := sdkmath.NewIntFromBigInt(expBigInt(rawBaseFee)); return &v }(),
					}, nil).Once()
			},
			expBaseFee: expBigInt(rawBaseFee),
		},
		{
			name: "gRPC error — fallback picks up new evm_fee_market event",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 2,
				FinalizeBlockEvents: []abcitypes.Event{
					{
						Type: evmtypes.EventTypeFeeMarket, // "evm_fee_market"
						Attributes: []abcitypes.EventAttribute{
							{Key: "base_fee", Value: rawBaseFee},
						},
					},
				},
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("state pruned")).Once()
			},
			expBaseFee: expBigInt(rawBaseFee),
		},
		{
			name: "gRPC error — fallback picks up legacy fee_market event",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 3,
				FinalizeBlockEvents: []abcitypes.Event{
					{
						Type: feemarkettypes.EventTypeFeeMarket, // "fee_market"
						Attributes: []abcitypes.EventAttribute{
							{Key: "base_fee", Value: rawBaseFee},
						},
					},
				},
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("state pruned")).Once()
			},
			expBaseFee: expBigInt(rawBaseFee),
		},
		{
			name: "gRPC error — no matching event returns nil with error",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 4,
				FinalizeBlockEvents: []abcitypes.Event{
					{
						Type: "unrelated_event",
						Attributes: []abcitypes.EventAttribute{
							{Key: "base_fee", Value: rawBaseFee},
						},
					},
				},
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("state pruned")).Once()
			},
			expBaseFee:    nil,
			expErrContain: "state pruned",
		},
		{
			name: "gRPC success with nil BaseFee field — returns nil",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 5,
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(&evmtypes.QueryBaseFeeResponse{BaseFee: nil}, nil).Once()
			},
			expBaseFee: nil,
		},
		{
			name: "gRPC error — fallback prefers last matching event (reverse scan)",
			blockRes: &tmrpctypes.ResultBlockResults{
				Height: 6,
				FinalizeBlockEvents: []abcitypes.Event{
					{
						Type: feemarkettypes.EventTypeFeeMarket,
						Attributes: []abcitypes.EventAttribute{
							{Key: "base_fee", Value: "111"},
						},
					},
					{
						Type: evmtypes.EventTypeFeeMarket,
						Attributes: []abcitypes.EventAttribute{
							{Key: "base_fee", Value: "999"},
						},
					},
				},
			},
			mockSetup: func(m *mocks.EVMQueryClient) {
				m.On("BaseFee", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, fmt.Errorf("pruned")).Once()
			},
			// reverse scan hits the second (evm_fee_market) event first
			expBaseFee: big.NewInt(999),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			backend := setupMockBackend(t)
			mockEVMQueryClient := backend.QueryClient.QueryClient.(*mocks.EVMQueryClient)
			tc.mockSetup(mockEVMQueryClient)

			result, err := backend.BaseFee(tc.blockRes)

			if tc.expErrContain != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrContain)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expBaseFee, result)
		})
	}
}
