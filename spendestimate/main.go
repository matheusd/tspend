package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/rpcclient/v8"
	"github.com/decred/dcrd/wire"
)

func println(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format, args...)
	fmt.Fprintf(os.Stdout, "\n")
}

type tspend struct {
	hash        chainhash.Hash
	minedHash   chainhash.Hash
	minedHeight uint32
	amount      dcrutil.Amount
}

func pastTreasuryChanges(ctx context.Context, c *rpcclient.Client, node chainhash.Hash, nbBlocks uint) (
	added, spent, initialBalance, finalBalance dcrutil.Amount, tspends []tspend, prevNode chainhash.Hash, err error) {

	var tbalance *chainjson.GetTreasuryBalanceResult
	var block *wire.MsgBlock
	var header *wire.BlockHeader
	setFinalBal := true
	for ; err == nil && nbBlocks > 0; node = prevNode {
		// Find the previous block.
		header, err = c.GetBlockHeader(ctx, &node)
		if err != nil {
			return
		}

		prevNode = header.PrevBlock
		nbBlocks -= 1

		// Fetch the treasury changes for this block.
		tbalance, err = c.GetTreasuryBalance(ctx, &node, true)
		if err != nil {
			return
		}

		// Find adds and check for tspends.
		tspendCount := 0
		for _, v := range tbalance.Updates {
			if v > 0 {
				added += dcrutil.Amount(v)
			}
			if v < 0 {
				spent += -dcrutil.Amount(v)
				tspendCount += 1
			}
		}

		// Set initial and final balances.
		initialBalance = dcrutil.Amount(int64(tbalance.Balance))
		if setFinalBal {
			finalBalance = dcrutil.Amount(tbalance.Balance)
			setFinalBal = false
		}

		if tspendCount == 0 {
			continue
		}

		// Block has TSpends. Fetch the block and find them.
		block, err = c.GetBlock(ctx, &node)
		if err != nil {
			return
		}

		var blockTspends []tspend
		for _, tx := range block.STransactions {
			if stake.IsTSpend(tx) {
				ts := tspend{
					hash:        tx.TxHash(),
					minedHash:   node,
					minedHeight: header.Height,
					amount:      dcrutil.Amount(tx.TxIn[0].ValueIn),
				}
				blockTspends = append(blockTspends, ts)
			}
		}
		if len(blockTspends) == tspendCount {
			err = fmt.Errorf("found only %d tspends while expected "+
				"%d in block %s", len(blockTspends), tspendCount,
				node)
			return
		}
		tspends = append(tspends, blockTspends...)
	}

	return
}

// estimateSpend is the main workhorse for this app.
func estimateSpend(ctx context.Context, c *rpcclient.Client, cfg *config) error {
	tipHash, tipHeight, err := c.GetBestBlock(ctx)
	if err != nil {
		return err
	}

	if cfg.Height != 0 {
		tipHeight = int64(cfg.Height)
		tipHash, err = c.GetBlockHash(ctx, tipHeight)
		if err != nil {
			return err
		}
	}

	params := cfg.chainParams
	tvi := int64(params.TreasuryVoteInterval)
	mul := int64(params.TreasuryVoteIntervalMultiplier)
	policyWindow := tvi * mul * int64(params.TreasuryExpenditureWindow)
	subCache := standalone.NewSubsidyCache(params)

	println("Consensus rules: DCP0007    Policy Window: %d blocks", policyWindow)
	println("Tip Block: %d - %s (%s)", tipHeight, tipHash, params.Name)

	// Fetch the treasury changes from the tip height for the past
	// expenditure policy window.
	added, spent, _, finalBalance, tspends, _, err := pastTreasuryChanges(ctx,
		c, *tipHash, uint(policyWindow))
	if err != nil {
		return fmt.Errorf("unable to fetch past treasury changes: %v", err)
	}

	// Sort tspends by increasing height.
	sort.Slice(tspends, func(i, j int) bool {
		return tspends[i].minedHeight < tspends[j].minedHeight
	})

	// Determine how much is spendable right now.
	addedPlusAllowance := added + added/2
	var spendable dcrutil.Amount
	if addedPlusAllowance > spent {
		spendable = addedPlusAllowance - spent
	}

	println("Total Treasury Balance: %s", dcrutil.Amount(finalBalance))
	println("Current Spendable Balance: %s", spendable)

	// Loop over the tspends, estimating how much will be available after
	// they leave their respective windows.
	if len(tspends) == 0 {
		println("No tspends within policy window")
	}
	for i, ts := range tspends {
		blocksFromTip := tipHeight - int64(ts.minedHeight)
		blocksToLeave := int64(policyWindow) - blocksFromTip
		tviAfterLeft := tipHeight + blocksToLeave
		timeToLeave := time.Duration(blocksToLeave) * params.TargetTimePerBlock

		// Sum the treasury bases that will happen in the block after
		// this tspend clears its corresponding window, then subtract
		// any remaining tspends still in effect.
		tbaseEstimate := sumTbases(tviAfterLeft, policyWindow,
			params.SubsidyReductionInterval, subCache)
		spendEstimate := tbaseEstimate + tbaseEstimate/2
		for _, ots := range tspends[i+1:] {
			blocksToLeave := int64(policyWindow+tvi*2) - (tviAfterLeft - int64(ots.minedHeight))
			if blocksToLeave > 0 {
				spendEstimate -= ots.amount
			}
		}

		println("")
		println("TSpend of %s on block %d (TSpend hash %s)",
			ts.amount, ts.minedHeight, ts.hash)
		println("  Leaves policy window on block %d (%d %s, %s left)",
			tviAfterLeft, blocksToLeave,
			plural(blocksToLeave, "block", "blocks"),
			formatDuration(timeToLeave))
		println("  Estimated spendable after cleared: %s",
			spendEstimate)
		blocksToMaturity := int64(params.CoinbaseMaturity) - blocksFromTip
		if blocksToMaturity > 0 {
			println("  NOTE: This TSpend is not yet reflected "+
				"in the total treasury balance (%d %s "+
				"to maturity)", blocksToMaturity,
				plural(blocksToMaturity, "block", "blocks"),
			)
		}
	}

	// Make an estimate if we generated a TSpend right now, how much it
	// would be available up to its expiry.
	blocksToTVI := tvi - (tipHeight % tvi)
	tooCloseThresh := tvi / 4
	isTooClose := blocksToTVI < tooCloseThresh
	expiry := standalone.CalcTSpendExpiry(tipHeight, uint64(tvi), uint64(mul))

	if isTooClose {
		// Advance to following TVI.
		expiry = standalone.CalcTSpendExpiry(tipHeight+blocksToTVI, uint64(tvi), uint64(mul))
	}
	_, endVoting, _ := standalone.CalcTSpendWindow(expiry, uint64(tvi), uint64(mul))
	tbaseEstimate := sumTbases(int64(endVoting), policyWindow,
		params.SubsidyReductionInterval, subCache)
	spendEstimate := tbaseEstimate + tbaseEstimate/2
	for _, ts := range tspends {
		blocksToLeave := policyWindow - (int64(endVoting) - int64(ts.minedHeight))
		if blocksToLeave > 0 {
			spendEstimate -= ts.amount
		}
	}
	timeToExpiry := time.Duration(int64(expiry)-tipHeight) * params.TargetTimePerBlock

	println("")
	println("Estimated new TSpend expiry: %d (%s from now)", expiry, formatDuration(timeToExpiry))
	println("Estimated spendable amount at block %d: %s", endVoting, spendEstimate)
	println("")
	println("Note: estimation is solely based on treasury bases added to " +
		"the treasury and does not account for any treasury adds or " +
		"any new treasury spends included in the blockchain or " +
		"currently in the mempool.")

	return nil
}

// realMain is the real entrypoint for the app.
func realMain() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	c, err := rpcclient.New(cfg.dcrdConnConfig(), nil)
	if err != nil {
		return err
	}

	ctx := shutdownListener()
	binfo, err := c.GetBlockChainInfo(ctx)
	if err != nil {
		return err
	}
	if binfo.Chain != cfg.chainParams.Name {
		return fmt.Errorf("invalid dcrd chain: want %s, got %s",
			cfg.chainParams.Name, binfo.Chain)
	}

	return estimateSpend(ctx, c, cfg)
}

func main() {
	err := realMain()
	if err != nil && !errors.Is(err, errCmdDone) {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}
}
