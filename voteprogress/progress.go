package main

import (
	"context"
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/rpcclient/v7"
)

func divCeil(a, b uint64) uint64 {
	r := a / b
	if a%b > 0 {
		r += 1
	}
	return r
}

func progressForTspend(params *chaincfg.Params, curBlockHeight uint64,
	tspend chainjson.TreasurySpendVotes) error {

	yesVotes := uint64(tspend.YesVotes)
	noVotes := uint64(tspend.NoVotes)
	voteStartBlock := uint64(tspend.VoteStart)
	voteEndBlock := uint64(tspend.VoteEnd)
	numVotesCast := yesVotes + noVotes
	var yesShare float64
	if numVotesCast > 0 {
		yesShare = float64(yesVotes) / float64(numVotesCast) * 100
	}

	fmt.Printf("\nTSpend %s\n", tspend.Hash)
	fmt.Printf("Votes Yes: %d  (%.2f%%) No: %d   Vote Interval: %d - %d\n",
		yesVotes, yesShare, noVotes, voteStartBlock, voteEndBlock)

	if curBlockHeight <= voteStartBlock {
		blocksToStart := voteStartBlock - curBlockHeight
		fmt.Printf("Voting hasn't started yet (%d blocks to start)\n", blocksToStart)
		return nil
	}

	votesPerBlock := uint64(params.VotesPerBlock())
	tvrMul := uint64(params.TreasuryVoteRequiredMultiplier)
	tvrDiv := uint64(params.TreasuryVoteRequiredDivisor)
	tvqMul := uint64(params.TreasuryVoteQuorumMultiplier)
	tvqDiv := uint64(params.TreasuryVoteQuorumDivisor)
	blocksPerDay := uint64(24 * 60 * 60 / params.TargetTimePerBlock.Seconds())
	tvi := uint64(params.TreasuryVoteInterval)

	// Vote Progress Calc.
	progress := float64(curBlockHeight-voteStartBlock) / float64(voteEndBlock-voteStartBlock) * 100
	fmt.Printf("Cast Votes: %d    Voting Progress: %2.f%%\n", numVotesCast, progress)

	// Participation calc.
	maxVotesToCurBlock := (curBlockHeight - voteStartBlock) * votesPerBlock
	participation := float64(numVotesCast) / float64(maxVotesToCurBlock) * 100

	fmt.Printf("Possible votes so far: %d    Participation: %.2f%%\n",
		maxVotesToCurBlock, participation)

	// Quorum Calc.
	maxVotes := votesPerBlock * (voteEndBlock - voteStartBlock)
	quorum := maxVotes * tvqMul / tvqDiv
	hasQuorum := numVotesCast >= quorum

	fmt.Printf("Max votes: %d   Quorum: %d   Has Quorum: %v\n", maxVotes, quorum, hasQuorum)

	// Shortcut Calc.

	remainingBlocks := voteEndBlock - curBlockHeight
	maxRemainingVotes := remainingBlocks * votesPerBlock
	daysToVoteEnd := float64(remainingBlocks) / float64(blocksPerDay)

	fmt.Printf("Blocks to end of voting: %d (%.2f days)    Max Remaining Votes %d\n",
		remainingBlocks, daysToVoteEnd, maxRemainingVotes)

	requiredYesVotes := (numVotesCast + maxRemainingVotes) * tvrMul / tvrDiv
	var missingYesVotes uint64
	if requiredYesVotes > yesVotes {
		missingYesVotes = requiredYesVotes - yesVotes
	}

	if missingYesVotes == 0 {
		approvalBlock := curBlockHeight
		inclusionBlock := approvalBlock + (tvi - approvalBlock%tvi)
		blocksToInclusion := inclusionBlock - curBlockHeight
		daysToInclusion := float64(blocksToInclusion) / float64(blocksPerDay)
		fmt.Printf("Tspend Approved!\n")
		fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n",
			blocksToInclusion, inclusionBlock, daysToInclusion)
		return nil
	}

	fmt.Printf("Required Yes Votes: %d     Missing Yes Votes: %d\n", requiredYesVotes, missingYesVotes)

	if missingYesVotes > maxRemainingVotes {
		fmt.Printf("Tspend Disapproved!\n")
		return nil
	}

	// Scenario 1: every vote from now on is a yes. So there are
	// votesPerBlock yes votes on every next block.

	blocksToReqVotes := divCeil(missingYesVotes, votesPerBlock)
	daysToReqVotes := float64(blocksToReqVotes) / float64(blocksPerDay)
	approvalBlock := curBlockHeight + blocksToReqVotes
	inclusionBlock := approvalBlock + (tvi - approvalBlock%tvi)
	blocksToInclusion := inclusionBlock - curBlockHeight
	daysToInclusion := float64(blocksToInclusion) / float64(blocksPerDay)

	fmt.Printf("\nScenario 1 - Every possible vote is cast as yes vote from now on\n")
	fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n",
		blocksToReqVotes, approvalBlock, daysToReqVotes)
	fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n",
		blocksToInclusion, inclusionBlock, daysToInclusion)

	// Scenario 2: Participation level remains constant, so there are
	// votesPerBlock * participation * yesFraction per block (on average).

	fmt.Printf("\nScenario 2 - Yes votes at current participation and approval levels\n")

	fracYesVotes := float64(yesVotes)
	cbh := curBlockHeight
	rmb := remainingBlocks
	for uint64(fracYesVotes) < requiredYesVotes {
		cbh += 1
		fracYesVotes += float64(votesPerBlock) * (participation / 100) * (yesShare / 100)

		// Recalc shortcut threshold.
		rmb = voteEndBlock - cbh
		maxRemainingVotes = rmb * votesPerBlock
		requiredYesVotes = (noVotes + uint64(fracYesVotes) + maxRemainingVotes) * tvrMul / tvrDiv
	}

	blocksToReqVotes = cbh - curBlockHeight
	daysToReqVotes = float64(blocksToReqVotes) / float64(blocksPerDay)
	approvalBlock = curBlockHeight + blocksToReqVotes
	inclusionBlock = approvalBlock + (tvi - approvalBlock%tvi)
	blocksToInclusion = inclusionBlock - curBlockHeight
	daysToInclusion = float64(blocksToInclusion) / float64(blocksPerDay)

	if blocksToReqVotes < remainingBlocks {
		fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n",
			blocksToReqVotes, approvalBlock, daysToReqVotes)
		fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n",
			blocksToInclusion, inclusionBlock, daysToInclusion)
	} else {
		fmt.Printf("  Impossible to approve tspend in this scenario\n")
	}

	// Scenario 3: No more votes come in until the end of voting.
	fmt.Printf("\nScenario 3 - No more votes come in\n")

	thresholdYesVotes := (yesVotes * tvrDiv / tvrMul)
	if thresholdYesVotes > numVotesCast {

		// Figure out how many remaining votes are too few to sway the
		// results and calc how many blocks worth of votes that is.

		thresholdRemainingVotes := thresholdYesVotes - yesVotes - noVotes
		thresholdBlock := voteEndBlock - (thresholdRemainingVotes / votesPerBlock)

		blocksToReqVotes = thresholdBlock - curBlockHeight
		daysToReqVotes = float64(blocksToReqVotes) / float64(blocksPerDay)
		approvalBlock = curBlockHeight + blocksToReqVotes
		inclusionBlock = approvalBlock + (tvi - approvalBlock%tvi)
		blocksToInclusion = inclusionBlock - curBlockHeight
		daysToInclusion = float64(blocksToInclusion) / float64(blocksPerDay)

		fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n",
			blocksToReqVotes, approvalBlock, daysToReqVotes)
		fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n",
			blocksToInclusion, inclusionBlock, daysToInclusion)
	} else {
		fmt.Printf("  Impossible to approve tspend in this scenario\n")
	}
	return nil

}

func voteProgress(cfg *config, ctx context.Context) error {
	connCfg := cfg.dcrdConnConfig()
	connCfg.DisableConnectOnNew = true
	connCfg.DisableAutoReconnect = true
	connCfg.HTTPPostMode = false
	c, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return fmt.Errorf("unable to create dcrd client: %v", err)
	}

	connCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = c.Connect(connCtx, false)
	if err != nil {
		return fmt.Errorf("unable to connect to dcrd: %v", err)
	}

	tspends, err := c.GetTreasurySpendVotes(ctx, nil, nil)
	if err != nil {
		return fmt.Errorf("unable to fetch tspends: %v", err)
	}

	if len(tspends.Votes) == 0 {
		return fmt.Errorf("no tspends in dcrd mempool")
	}

	fmt.Printf("Checking %d tspends at block %d (%s)\n", len(tspends.Votes),
		tspends.Height, tspends.Hash)

	for i, tspend := range tspends.Votes {
		err := progressForTspend(cfg.chainParams, uint64(tspends.Height), tspend)
		if err != nil {
			return fmt.Errorf("error processing tspend %d: %v", i, err)
		}
	}

	return nil
}
