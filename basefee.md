# BaseFee Nil Pointer Dereference on Legacy Blocks

## Observed Error

```
failed to fetch Base Fee from prunned block. Check node prunning configuration
error="rpc error: code = Internal desc = runtime error: invalid memory address or nil pointer dereference"
```

This surfaces when querying any JSON-RPC method that resolves a base fee for a
block that was originally produced by an Ethermint node (legacy chain state).

---

## How BaseFee Is Resolved

`BaseFee(blockRes)` in `rpc/backend/chain_info.go` has two steps:

1. **gRPC query** — `QueryClient.BaseFee` is called at the block's height. For
   legacy blocks the state root belongs to the old Ethermint store layout, so the
   EVM keeper's `GetBaseFee` is called inside `FeeMarketWrapper.GetBaseFee`, which
   panics (see Root Cause A). gRPC catches the panic and returns an `Internal`
   error to the client.

2. **Event fallback** — when the gRPC call fails the code iterates
   `FinalizeBlockEvents` looking for a `"evm_fee_market"` event. Legacy blocks
   emit `"fee_market"` instead, so no match is found and the fallback returns
   `nil, err` (Root Cause B). A second identical gap exists in
   `BaseFeeFromEvents` in `rpc/types/utils.go` (Root Cause C).

---

## Root Cause A — Nil pointer panic in `x/vm/wrappers/feemarket.go`

### What happens

```go
// Before fix
func (w FeeMarketWrapper) GetBaseFee(ctx sdk.Context, decimals types.Decimals) *big.Int {
    baseFee := w.FeeMarketKeeper.GetBaseFee(ctx)
    if baseFee.IsNil() {
        return nil
    }
    return baseFee.MulInt(decimals.ConversionFactor()).TruncateInt().BigInt()
    //                    ^^^^^^^^^^^^^^^^^^^^^^^^^^^ can return math.Int{i: nil}
}
```

`decimals.ConversionFactor()` is a map lookup:

```go
// x/vm/types/denom.go
var ConversionFactor = map[Decimals]math.Int{
    OneDecimals:   math.NewInt(1e17),
    // ...entries for 1–18 only...
}

func (d Decimals) ConversionFactor() math.Int {
    return ConversionFactor[d]  // missing key → zero value of math.Int
}
```

`math.Int` is a thin wrapper around `*big.Int`. The zero value of a struct in Go
has all fields at their zero value, so a missing map key returns
`math.Int{i: nil}` — an `Int` whose inner pointer is `nil`.

For a legacy block, `GetEvmCoinInfo` cannot find chain params in the new store
and falls back to `k.defaultEvmCoinInfo`, which is the zero value of the struct:
`Decimals = 0`. `0` is not a key in `ConversionFactor`, so the lookup returns
`math.Int{i: nil}`.

Calling `baseFee.MulInt(math.Int{i: nil})` eventually reaches `big.Int.Mul(x,
nil)`, which panics.

### Call chain

```
rpc/backend/chain_info.go: BaseFee()
  → QueryClient.BaseFee (gRPC)
    → x/vm/keeper: QueryBaseFee
      → FeeMarketWrapper.GetBaseFee(ctx, GetEvmCoinInfo(ctx).Decimals)
        → decimals = 0  (legacy block, store miss → default zero value)
        → ConversionFactor[0] → math.Int{i: nil}
        → baseFee.MulInt(nil_int) → big.Int.Mul(x, nil) → PANIC
```

### Fix

```go
// x/vm/wrappers/feemarket.go
factor := decimals.ConversionFactor()
if factor.IsNil() {
    // Decimals is 0 or out-of-range (legacy block).
    // The feemarket keeper already stores the base fee in 18-decimal form,
    // so no scaling is needed.
    return baseFee.TruncateInt().BigInt()
}
return baseFee.MulInt(factor).TruncateInt().BigInt()
```

---

## Root Cause B — Event type mismatch in `rpc/backend/chain_info.go`

The gRPC fallback scans `FinalizeBlockEvents`:

```go
// Before fix
if evt.Type == evmtypes.EventTypeFeeMarket && len(evt.Attributes) > 0 {
```

| Module | Constant | Value |
|---|---|---|
| `x/vm/types` | `EventTypeFeeMarket` | `"evm_fee_market"` |
| `x/feemarket/types` | `EventTypeFeeMarket` | `"fee_market"` |

Legacy Ethermint nodes emit `"fee_market"`. The condition only checked for
`"evm_fee_market"`, so legacy events were silently skipped and the fallback
returned `nil, err`.

### Fix

```go
// rpc/backend/chain_info.go
if (evt.Type == evmtypes.EventTypeFeeMarket || evt.Type == feemarkettypes.EventTypeFeeMarket) && len(evt.Attributes) > 0 {
```

---

## Root Cause C — Same event type mismatch in `rpc/types/utils.go`

`BaseFeeFromEvents` has the same single-type guard:

```go
// Before fix
if event.Type != evmtypes.EventTypeFeeMarket {
    continue
}
```

This function is called from several RPC paths that parse events directly (e.g.
`eth_getLogs`, stream subscriptions). Legacy `"fee_market"` events were
discarded, returning `nil` base fee.

### Fix

```go
// rpc/types/utils.go
if event.Type != evmtypes.EventTypeFeeMarket && event.Type != feemarkettypes.EventTypeFeeMarket {
    continue
}
```

---

## Files Changed

| File | Change |
|---|---|
| `x/vm/wrappers/feemarket.go` | Guard `ConversionFactor()` nil before `MulInt`; return raw truncated value for legacy decimals |
| `rpc/backend/chain_info.go` | Accept both `"evm_fee_market"` and `"fee_market"` in the event fallback loop |
| `rpc/types/utils.go` | Accept both event types in `BaseFeeFromEvents` |
