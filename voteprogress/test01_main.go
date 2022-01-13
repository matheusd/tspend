package main

import (
	"fmt"
)

func __main() {
	yesVotes := uint64(8478)
	noVotes := uint64(0)
	curBlockHeight := uint64(564352)
	voteStartBlock := uint64(561888)
	voteEndBlock := uint64(565344)

	votesPerBlock := uint64(5)
	tvrMul := uint64(3)
	tvrDiv := uint64(5)
	tvqMul := uint64(1)
	tvqDiv := uint64(5)
	blocksPerDay := uint64(288)
	tvi := uint64(288)

	// Vote Progress Calc.
	numVotesCast := yesVotes + noVotes
	progress := float64(curBlockHeight-voteStartBlock) / float64(voteEndBlock-voteStartBlock) * 100
	fmt.Printf("Cast Votes: %d    Voting Progress: %2.f%%\n", numVotesCast, progress)

	// Participation calc.
	maxVotesToCurBlock := (curBlockHeight - voteStartBlock) * votesPerBlock
	participation := float64(numVotesCast) / float64(maxVotesToCurBlock) * 100

	fmt.Printf("Possible votes so far: %d    Participation: %.2f%%\n", maxVotesToCurBlock, participation)

	// Quorum Calc.
	maxVotes := votesPerBlock * (voteEndBlock - voteStartBlock)
	quorum := maxVotes * tvqMul / tvqDiv
	hasQuorum := numVotesCast >= quorum

	fmt.Printf("Max votes: %d   Quorum: %d   Has Quorum: %v\n", maxVotes, quorum, hasQuorum)

	// Shortcut Calc.

	remainingBlocks := voteEndBlock - curBlockHeight
	maxRemainingVotes := remainingBlocks * votesPerBlock

	fmt.Printf("Blocks to end of voting: %d    Max Remaining Votes %d\n", remainingBlocks, maxRemainingVotes)

	requiredYesVotes := (numVotesCast + maxRemainingVotes) * tvrMul / tvrDiv
	var missingYesVotes uint64
	if requiredYesVotes > yesVotes {
		missingYesVotes = requiredYesVotes - yesVotes
	}

	fmt.Printf("Required Yes Votes: %d     Missing Yes Votes: %d\n", requiredYesVotes, missingYesVotes)

	// Scenario 1: every vote from now on is a yes. So there are votesPerBlock yes votes on every next block.
	blocksToReqVotes := divCeil(missingYesVotes, votesPerBlock)
	daysToReqVotes := float64(blocksToReqVotes) / float64(blocksPerDay)
	approvalBlock := curBlockHeight + blocksToReqVotes
	inclusionBlock := approvalBlock + (tvi - approvalBlock%tvi)
	blocksToInclusion := inclusionBlock - curBlockHeight
	daysToInclusion := float64(blocksToInclusion) / float64(blocksPerDay)

	fmt.Printf("\nScenario 1 - Every possible vote is cast as yes vote from now on\n")
	fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n", blocksToReqVotes, approvalBlock, daysToReqVotes)
	fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n", blocksToInclusion, inclusionBlock, daysToInclusion)

	fmt.Printf("\nScenario 2 - Only yes votes at current participation level\n")

	fracYesVotes := float64(yesVotes)
	cbh := curBlockHeight
	for uint64(fracYesVotes) < requiredYesVotes {
		cbh += 1
		fracYesVotes += float64(votesPerBlock) * participation / 100

		// Recalc shortcut threshold.
		remainingBlocks = voteEndBlock - cbh
		maxRemainingVotes = remainingBlocks * votesPerBlock
		requiredYesVotes = (noVotes + uint64(fracYesVotes) + maxRemainingVotes) * tvrMul / tvrDiv
	}

	blocksToReqVotes = cbh - curBlockHeight
	daysToReqVotes = float64(blocksToReqVotes) / float64(blocksPerDay)
	approvalBlock = curBlockHeight + blocksToReqVotes
	inclusionBlock = approvalBlock + (tvi - approvalBlock%tvi)
	blocksToInclusion = inclusionBlock - curBlockHeight
	daysToInclusion = float64(blocksToInclusion) / float64(blocksPerDay)

	fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n", blocksToReqVotes, approvalBlock, daysToReqVotes)
	fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n", blocksToInclusion, inclusionBlock, daysToInclusion)

	fmt.Printf("\nScenario 3 - No more votes come in\n")

	// Figure out how many remaining votes are too few to sway the results and
	// calc how many blocks worth of votes that is.
	thresholdRemainingVotes := (yesVotes * tvrDiv / tvrMul) - yesVotes - noVotes
	thresholdBlock := voteEndBlock - (thresholdRemainingVotes / votesPerBlock)

	blocksToReqVotes = thresholdBlock - curBlockHeight
	daysToReqVotes = float64(blocksToReqVotes) / float64(blocksPerDay)
	approvalBlock = curBlockHeight + blocksToReqVotes
	inclusionBlock = approvalBlock + (tvi - approvalBlock%tvi)
	blocksToInclusion = inclusionBlock - curBlockHeight
	daysToInclusion = float64(blocksToInclusion) / float64(blocksPerDay)

	fmt.Printf("  Blocks to Shortcut Approval: %d (block %d)     Days %.2f\n", blocksToReqVotes, approvalBlock, daysToReqVotes)
	fmt.Printf("  Blocks to inclusion oportunity: %d (block %d)   Days To Oportunity: %.2f\n", blocksToInclusion, inclusionBlock, daysToInclusion)
}
