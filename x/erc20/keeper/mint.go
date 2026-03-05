package keeper

import (
	"github.com/cosmos/evm/x/erc20/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// MintingEnabled checks that:
//   - the global parameter for erc20 conversion is enabled
//   - minting is enabled for the given (erc20,coin) token pair
//   - recipient address is not on the blocked list
//   - bank module transfers are enabled for the Cosmos coin
func (k Keeper) MintingEnabled(
	ctx sdk.Context,
	receiver sdk.AccAddress,
	token string,
) (types.TokenPair, error) {
	if !k.IsERC20Enabled(ctx) {
		return types.TokenPair{}, errorsmod.Wrap(
			types.ErrERC20Disabled, "module is currently disabled by governance",
		)
	}

	id := k.GetTokenPairID(ctx, token)
	if len(id) == 0 {
		return types.TokenPair{}, errorsmod.Wrapf(
			types.ErrTokenPairNotFound, "token '%s' not registered by id", token,
		)
	}

	pair, found := k.GetTokenPair(ctx, id)
	if !found {
		return types.TokenPair{}, errorsmod.Wrapf(
			types.ErrTokenPairNotFound, "token '%s' not registered", token,
		)
	}

	if !pair.Enabled {
		return types.TokenPair{}, errorsmod.Wrapf(
			types.ErrERC20TokenPairDisabled, "minting token '%s' is not enabled by governance", token,
		)
	}

	if k.bankKeeper.BlockedAddr(receiver.Bytes()) {
		return types.TokenPair{}, errorsmod.Wrapf(
			errortypes.ErrUnauthorized, "%s is not allowed to receive transactions", receiver,
		)
	}

	// IsSendEnabledCoin only checks the denom, not the amount, so we construct a
	// placeholder coin with amount=1 to satisfy the sdk.Coin interface without
	// altering the function signature.
	coin := sdk.Coin{Denom: pair.Denom, Amount: math.OneInt()}

	// check if minting to a recipient address other than the sender is enabled
	// for for the given coin denom
	if !k.bankKeeper.IsSendEnabledCoin(ctx, coin) {
		return types.TokenPair{}, errorsmod.Wrapf(
			banktypes.ErrSendDisabled, "minting '%s' coins to an external address is currently disabled", token,
		)
	}

	return pair, nil
}

// MintCoins mints the provided amount of coins to the given address.
func (k Keeper) MintCoins(ctx sdk.Context, sender, to sdk.AccAddress, amount math.Int, token string) error {
	pair, err := k.MintingEnabled(ctx, to, token)
	if err != nil {
		return err
	}

	if !pair.IsNativeCoin() {
		return errorsmod.Wrap(types.ErrNonNativeCoinMintingDisabled, token)
	}

	contractOwnerAddr, err := sdk.AccAddressFromBech32(pair.OwnerAddress)
	if err != nil {
		return errorsmod.Wrapf(err, "invalid owner address")
	}
	if !sender.Equals(contractOwnerAddr) {
		return types.ErrMinterIsNotOwner
	}

	coins := sdk.Coins{{Denom: pair.Denom, Amount: amount}}
	err = k.bankKeeper.MintCoins(ctx, types.ModuleName, coins)
	if err != nil {
		return err
	}

	err = k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, to, coins)
	if err != nil {
		return err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyAction, types.TypeMsgMint),
			sdk.NewAttribute(sdk.AttributeKeyModule, types.ModuleName),
			sdk.NewAttribute(sdk.AttributeKeySender, sender.String()),
			sdk.NewAttribute(sdk.AttributeKeyAmount, amount.String()),
		),
	)

	return nil
}
