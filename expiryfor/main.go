package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	blockchain "github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s [--simnet|--testnet] [block height]\n", filepath.Base(os.Args[0]))
		fmt.Println("Find out the tspend expiry for a given block height")
		os.Exit(1)
	}

	chain := chaincfg.MainNetParams()
	heightStr := os.Args[1]
	if os.Args[1] == "--testnet" {
		chain = chaincfg.TestNet3Params()
		heightStr = os.Args[2]
	} else if os.Args[1] == "--simnet" {
		chain = chaincfg.SimNetParams()
		heightStr = os.Args[2]
	}

	height, err := strconv.ParseInt(heightStr, 10, 32)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	height += 1 // Assume height is mined, so start calc for height+1.
	tvi := chain.TreasuryVoteInterval
	mul := chain.TreasuryVoteIntervalMultiplier

	fmt.Printf("Chain: %s TVI %d MUL %d\n", chain.Name, tvi, mul)

	blocksToTVI := int64(tvi) - (height % int64(tvi))
	tooCloseThresh := int64(tvi / 4)
	isTooClose := blocksToTVI < tooCloseThresh

	isTVI := blockchain.IsTreasuryVoteInterval(uint64(height), tvi)
	expiry := blockchain.CalcTSpendExpiry(height, tvi, mul)
	start, end, _ := blockchain.CalcTSpendWindow(expiry, tvi, mul)

	fmt.Printf("Height %d: IsTVI: %v\n", height, isTVI)
	fmt.Printf("To TVI: %d (thresh %d)\n", blocksToTVI, tooCloseThresh)
	fmt.Printf("Expiry: %d\n", expiry)
	fmt.Printf("Voting interval: %d - %d\n", start, end)
	if isTooClose {
		// Advance to following TVI.
		height += blocksToTVI
		expiry = blockchain.CalcTSpendExpiry(height, tvi, mul)
		start, end, _ = blockchain.CalcTSpendWindow(expiry, tvi, mul)
		fmt.Println("\nHeight too close to TVI. Advancing to next one.")
		fmt.Printf("Expiry: %d\n", expiry)
		fmt.Printf("Voting interval: %d - %d\n", start, end)
	}
}
