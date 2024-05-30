package e2e

import (
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm/ibctesting"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	"github.com/babylonchain/babylon-sdk/demo/app"
	bstypes "github.com/babylonchain/babylon/x/btcstaking/types"
	zctypes "github.com/babylonchain/babylon/x/zoneconcierge/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func NewBTCStakingPacketData(packet *bstypes.BTCStakingIBCPacket) *zctypes.ZoneconciergePacketData {
	return &zctypes.ZoneconciergePacketData{
		Packet: &zctypes.ZoneconciergePacketData_BtcStaking{
			BtcStaking: packet,
		},
	}
}

// NewIBCCoordinator initializes Coordinator with N bcd TestChain instances
func NewIBCCoordinator(t *testing.T, opts ...[]wasmkeeper.Option) *ibctesting.Coordinator {
	return ibctesting.NewCoordinatorX(t, 2,
		func(
			t *testing.T,
			valSet *types.ValidatorSet,
			genAccs []authtypes.GenesisAccount,
			chainID string,
			opts []wasmkeeper.Option,
			balances ...banktypes.Balance,
		) ibctesting.ChainApp {
			return app.SetupWithGenesisValSet(t, valSet, genAccs, chainID, opts, balances...)
		},
		opts...,
	)
}

func InstantiateContract(t *testing.T, chain *ibctesting.TestChain, codeID uint64, initMsg []byte, funds ...sdk.Coin) sdk.AccAddress {
	instantiateMsg := &wasmtypes.MsgInstantiateContract{
		Sender: chain.SenderAccount.GetAddress().String(),
		Admin:  chain.SenderAccount.GetAddress().String(),
		CodeID: codeID,
		Label:  "ibc-test",
		Msg:    initMsg,
		Funds:  funds,
	}

	r, err := chain.SendMsgs(instantiateMsg)
	require.NoError(t, err)
	require.Zero(t, r.Code)
	require.NotEmpty(t, r.Data)

	// ensure there is only 1 contract under this code ID
	ctx := chain.GetContext()
	contractAddrs := []sdk.AccAddress{}
	chain.App.GetWasmKeeper().IterateContractsByCode(ctx, codeID, func(address sdk.AccAddress) bool {
		contractAddrs = append(contractAddrs, address)
		return false // keep iterating
	})
	require.Len(t, contractAddrs, 1)

	return contractAddrs[0]
}
