package types

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"

	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

func TestBaseFeeFromEvents(t *testing.T) {
	const baseFeeValue = "1000000000000000000" // 1e18

	testCases := []struct {
		name      string
		events    []abci.Event
		expResult *big.Int
	}{
		{
			name: "new evm_fee_market event type",
			events: []abci.Event{
				{
					Type: evmtypes.EventTypeFeeMarket, // "evm_fee_market"
					Attributes: []abci.EventAttribute{
						{Key: evmtypes.AttributeKeyBaseFee, Value: baseFeeValue},
					},
				},
			},
			expResult: func() *big.Int { b, _ := new(big.Int).SetString(baseFeeValue, 10); return b }(),
		},
		{
			name: "legacy fee_market event type",
			events: []abci.Event{
				{
					Type: feemarkettypes.EventTypeFeeMarket, // "fee_market"
					Attributes: []abci.EventAttribute{
						{Key: evmtypes.AttributeKeyBaseFee, Value: baseFeeValue},
					},
				},
			},
			expResult: func() *big.Int { b, _ := new(big.Int).SetString(baseFeeValue, 10); return b }(),
		},
		{
			name: "unrelated event type is ignored",
			events: []abci.Event{
				{
					Type: "some_other_event",
					Attributes: []abci.EventAttribute{
						{Key: evmtypes.AttributeKeyBaseFee, Value: baseFeeValue},
					},
				},
			},
			expResult: nil,
		},
		{
			name: "matching event type but wrong attribute key",
			events: []abci.Event{
				{
					Type: evmtypes.EventTypeFeeMarket,
					Attributes: []abci.EventAttribute{
						{Key: "not_base_fee", Value: baseFeeValue},
					},
				},
			},
			expResult: nil,
		},
		{
			name: "matching event type but invalid base_fee value",
			events: []abci.Event{
				{
					Type: feemarkettypes.EventTypeFeeMarket,
					Attributes: []abci.EventAttribute{
						{Key: evmtypes.AttributeKeyBaseFee, Value: "not_a_number"},
					},
				},
			},
			expResult: nil,
		},
		{
			name:      "empty event list",
			events:    []abci.Event{},
			expResult: nil,
		},
		{
			name: "legacy event comes after an unrelated event",
			events: []abci.Event{
				{Type: "coin_received", Attributes: []abci.EventAttribute{{Key: "amount", Value: "100"}}},
				{
					Type: feemarkettypes.EventTypeFeeMarket,
					Attributes: []abci.EventAttribute{
						{Key: evmtypes.AttributeKeyBaseFee, Value: "42"},
					},
				},
			},
			expResult: big.NewInt(42),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BaseFeeFromEvents(tc.events)
			require.Equal(t, tc.expResult, result)
		})
	}
}
