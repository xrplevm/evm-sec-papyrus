# Fix: Legacy Transaction Response Decoding (`eth_getBlockReceipts`)

## Problem

Calling `eth_getBlockReceipts` (or any RPC that fetches receipt/log data) on blocks containing
legacy Ethermint transactions fails with:

```
failed to get receipts from comet block: failed to convert tx result to eth receipt:
invalid message index: 0
```

## Root Cause

### Error chain

1. `eth_getBlockReceipts` → `EthBlockFromCometBlock` → `ReceiptsFromCometBlock` (`rpc/backend/comet_to_eth.go:278`)
2. Calls `evmtypes.DecodeMsgLogs(blockRes.TxsResults[i].Data, msgIndex, height)`
3. `DecodeMsgLogs` → `DecodeTxResponses` (`x/vm/types/utils.go:53–74`)
4. TypeUrl filter silently skips all legacy response entries:
   ```go
   if res.TypeUrl != "/"+proto.MessageName(&response) {
       continue  // ← silently skips "/ethermint.evm.v1.MsgEthereumTxResponse"
   }
   ```
5. New type URL = `/cosmos.evm.vm.v1.MsgEthereumTxResponse`
   Legacy type URL = `/ethermint.evm.v1.MsgEthereumTxResponse` → **no match**
6. All responses skipped → empty `txResponses` slice
7. Back in `DecodeMsgLogs`: `msgIndex(0) >= len(txResponses)(0)` → **"invalid message index: 0"**

### Proto wire-format compatibility

The legacy and new `MsgEthereumTxResponse` structs are **binary-compatible**:

| Field # | Legacy (`ethermint.evm.v1`) | New (`cosmos.evm.vm.v1`) |
|---|---|---|
| 1 | Hash (string) | Hash (string) |
| 2 | Logs ([]*Log) | Logs ([]*Log) |
| 3 | Ret (bytes) | Ret (bytes) |
| 4 | VmError (string) | VmError (string) |
| 5 | GasUsed (uint64) | GasUsed (uint64) |
| 6 | — | MaxUsedGas (uint64) → defaults to 0 |
| 7 | — | BlockHash (bytes) → defaults to nil |
| 8 | — | BlockTimestamp (uint64) → defaults to 0 |

Same for `Log`: legacy has fields 1–9, new adds field 10 (`BlockTimestamp`). Legacy bytes can
be decoded directly with the new type — unknown fields are safely ignored per proto3 semantics.

---

## Fix Plan

### Step 1 — Fix `DecodeTxResponses` in `x/vm/types/utils.go` (core fix)

Add a constant for the legacy TypeUrl and accept both in the TypeUrl filter.

**Current code (`x/vm/types/utils.go:62–65`):**
```go
for _, res := range txMsgData.MsgResponses {
    var response MsgEthereumTxResponse
    if res.TypeUrl != "/"+proto.MessageName(&response) {
        continue
    }
```

**Proposed change:**
```go
const legacyMsgEthereumTxResponseTypeURL = "/ethermint.evm.v1.MsgEthereumTxResponse"

func DecodeTxResponses(in []byte) ([]*MsgEthereumTxResponse, error) {
    if in == nil {
        return nil, nil
    }
    var txMsgData sdk.TxMsgData
    if err := proto.Unmarshal(in, &txMsgData); err != nil {
        return nil, err
    }
    var zeroResponse MsgEthereumTxResponse
    newTypeURL := "/" + proto.MessageName(&zeroResponse)

    responses := make([]*MsgEthereumTxResponse, 0, len(txMsgData.MsgResponses))
    for _, res := range txMsgData.MsgResponses {
        if res.TypeUrl != newTypeURL && res.TypeUrl != legacyMsgEthereumTxResponseTypeURL {
            continue
        }
        var response MsgEthereumTxResponse
        if err := proto.Unmarshal(res.Value, &response); err != nil {
            return nil, errorsmod.Wrap(err, "failed to unmarshal tx response message data")
        }
        responses = append(responses, &response)
    }
    return responses, nil
}
```

This fixes **all callers** at once:
- `rpc/backend/comet_to_eth.go` (ReceiptsFromCometBlock)
- `rpc/backend/tx_info.go`
- `precompiles/testutil/`, `testutil/integration/`, etc.

**File to modify:** `x/vm/types/utils.go`

---

### Step 2 — Create `rpc/backend/eth/response_adapter.go` (adapter pattern)

Mirror the request adapter (`rpc/backend/eth/adapter.go`) for the response side.
This provides explicit struct-level conversion from legacy → new type, following the
project's established architectural pattern and documenting the field mapping.

**New file:** `rpc/backend/eth/response_adapter.go`

```go
package eth

import (
    legacytypes "github.com/cosmos/evm/rpc/types/legacy"
    evmtypes "github.com/cosmos/evm/x/vm/types"
)

// AdaptTxResponse converts a legacy Ethermint MsgEthereumTxResponse to the new
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
            // BlockTimestamp (field 10) absent in legacy — defaults to 0
        }
    }
    return &evmtypes.MsgEthereumTxResponse{
        Hash:    legacy.Hash,
        Logs:    logs,
        Ret:     legacy.Ret,
        VmError: legacy.VmError,
        GasUsed: legacy.GasUsed,
        // MaxUsedGas, BlockHash, BlockTimestamp absent in legacy — default to zero
    }
}
```

---

## Files to Change

| File | Type | Change |
|---|---|---|
| `x/vm/types/utils.go` | Modify | Add `legacyMsgEthereumTxResponseTypeURL` constant; update TypeUrl filter in `DecodeTxResponses` |
| `rpc/backend/eth/response_adapter.go` | New | `AdaptTxResponse` struct-level converter (legacy → new) |

## Files Unchanged

| File | Reason |
|---|---|
| `rpc/backend/comet_to_eth.go` | Already calls `evmtypes.DecodeMsgLogs`; fixed by Step 1 |
| `rpc/backend/tx_info.go` | Same pattern; fixed by Step 1 |

---

## Verification

1. `go build ./...` — confirms no compilation errors
2. `go test ./x/vm/types/... -run TestDecodeTxResponses` — existing test suite passes
3. **New test case** in `x/vm/types/utils_test.go`: encode a `sdk.TxMsgData` with TypeUrl
   `/ethermint.evm.v1.MsgEthereumTxResponse` and verify `DecodeTxResponses` returns the
   decoded response (not empty slice)
4. Live: call `eth_getBlockReceipts` on a block that contained legacy Ethermint transactions
   and confirm no error
