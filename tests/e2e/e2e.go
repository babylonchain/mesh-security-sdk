package e2e

import (
	"path/filepath"
	"testing"

	"github.com/CosmWasm/wasmd/x/wasm"
	"github.com/CosmWasm/wasmd/x/wasm/ibctesting"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cometbft/cometbft/types"
	types2 "github.com/cosmos/ibc-go/v7/modules/core/04-channel/types"
	ibctesting2 "github.com/cosmos/ibc-go/v7/testing"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	"github.com/babylonchain/babylon-sdk/demo/app"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
)

var (
	wasmContractPath    string
	wasmContractGZipped bool
)

func buildPathToWasm(fileName string) string {
	if wasmContractGZipped {
		fileName += ".gz"
	}
	return filepath.Join(wasmContractPath, fileName)
}

// NewIBCCoordinator initializes Coordinator with N bcd TestChain instances
func NewIBCCoordinator(t *testing.T, opts ...[]wasmkeeper.Option) *ibctesting.Coordinator {
	return ibctesting.NewCoordinatorX(t, 2,
		func(t *testing.T, valSet *types.ValidatorSet, genAccs []authtypes.GenesisAccount, chainID string, opts []wasm.Option, balances ...banktypes.Balance) ibctesting.ChainApp {
			return app.SetupWithGenesisValSet(t, valSet, genAccs, chainID, opts, balances...)
		},
		opts...,
	)
}

func submitGovProposal(t *testing.T, chain *ibctesting.TestChain, msgs ...sdk.Msg) uint64 {
	chainApp := chain.App.(*app.ConsumerApp)
	govParams := chainApp.GovKeeper.GetParams(chain.GetContext())
	govMsg, err := govv1.NewMsgSubmitProposal(msgs, govParams.MinDeposit, chain.SenderAccount.GetAddress().String(), "", "my title", "my summary")
	require.NoError(t, err)
	rsp, err := chain.SendMsgs(govMsg)
	require.NoError(t, err)
	id := rsp.MsgResponses[0].GetCachedValue().(*govv1.MsgSubmitProposalResponse).ProposalId
	require.NotEmpty(t, id)
	return id
}

func voteAndPassGovProposal(t *testing.T, chain *ibctesting.TestChain, proposalID uint64) {
	vote := govv1.NewMsgVote(chain.SenderAccount.GetAddress(), proposalID, govv1.OptionYes, "testing")
	_, err := chain.SendMsgs(vote)
	require.NoError(t, err)

	chainApp := chain.App.(*app.ConsumerApp)
	govParams := chainApp.GovKeeper.GetParams(chain.GetContext())

	coord := chain.Coordinator
	coord.IncrementTimeBy(*govParams.VotingPeriod)
	coord.CommitBlock(chain)

	rsp, err := chainApp.GovKeeper.Proposal(sdk.WrapSDKContext(chain.GetContext()), &govv1.QueryProposalRequest{ProposalId: proposalID})
	require.NoError(t, err)
	require.Equal(t, rsp.Proposal.Status, govv1.ProposalStatus_PROPOSAL_STATUS_PASSED)
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
	require.Len(t, r.MsgResponses, 1)
	require.NotEmpty(t, r.MsgResponses[0].GetCachedValue())
	pExecResp := r.MsgResponses[0].GetCachedValue().(*wasmtypes.MsgInstantiateContractResponse)
	a, err := sdk.AccAddressFromBech32(pExecResp.Address)
	require.NoError(t, err)
	return a
}

type example struct {
	Coordinator      *ibctesting.Coordinator
	ConsumerChain    *ibctesting.TestChain
	ProviderChain    *ibctesting.TestChain
	ConsumerApp      *app.ConsumerApp
	IbcPath          *ibctesting.Path
	ProviderDenom    string
	ConsumerDenom    string
	MyProvChainActor string
}

func setupExampleChains(t *testing.T) example {
	coord := NewIBCCoordinator(t)
	provChain := coord.GetChain(ibctesting2.GetChainID(1))
	consChain := coord.GetChain(ibctesting2.GetChainID(2))
	return example{
		Coordinator:      coord,
		ConsumerChain:    consChain,
		ProviderChain:    provChain,
		ConsumerApp:      consChain.App.(*app.ConsumerApp),
		IbcPath:          ibctesting.NewPath(consChain, provChain),
		ProviderDenom:    sdk.DefaultBondDenom,
		ConsumerDenom:    sdk.DefaultBondDenom,
		MyProvChainActor: provChain.SenderAccount.GetAddress().String(),
	}
}

func setupBabylonIntegration(t *testing.T, x example) (*TestConsumerClient, ConsumerContract, *TestProviderClient) {
	x.Coordinator.SetupConnections(x.IbcPath)

	// setup contracts on both chains
	consumerCli := NewConsumerClient(t, x.ConsumerChain)
	consumerContracts := consumerCli.BootstrapContracts()
	consumerPortID := wasmkeeper.PortIDForContract(consumerContracts.Babylon)
	// add some fees so that we can distribute something
	x.ConsumerChain.DefaultMsgFees = sdk.NewCoins(sdk.NewCoin(x.ConsumerDenom, math.NewInt(1_000_000)))

	providerCli := NewProviderClient(t, x.ProviderChain)

	return consumerCli, consumerContracts, providerCli

	// TODO: fix IBC channel below

	// setup ibc path
	x.IbcPath.EndpointB.ChannelConfig = &ibctesting2.ChannelConfig{
		PortID: "zoneconcierge", // TODO: replace this chain/port with Babylon
		Order:  types2.ORDERED,
	}
	x.IbcPath.EndpointA.ChannelConfig = &ibctesting2.ChannelConfig{
		PortID: consumerPortID,
		Order:  types2.ORDERED,
	}
	x.Coordinator.CreateChannels(x.IbcPath)

	// when ibc package is relayed
	require.NotEmpty(t, x.ConsumerChain.PendingSendPackets)
	require.NoError(t, x.Coordinator.RelayAndAckPendingPackets(x.IbcPath))

	return consumerCli, consumerContracts, providerCli
}
