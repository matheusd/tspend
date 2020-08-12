package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/blockchain/stake/v3"
	blockchain "github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/rpcclient/v6"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"golang.org/x/crypto/ssh/terminal"
)

// tspend_sigscript_size is the size of a tspend sigscript:
// OP_DATA_65 + [64 byte schnorr sig + sighashtype byte ] + OP_DATA_33 + [33 byte pubkey]
const tspend_sigscript_size int = 1 + 65 + 1 + 33

func privKeyFromStdIn(pk *[32]byte) error {
	var hexPk []byte
	var err error
	fd := int(os.Stdin.Fd())
	if terminal.IsTerminal(fd) {
		fmt.Print("Input the private key: ")
		hexPk, err = terminal.ReadPassword(fd)
	} else {
		r := bufio.NewReader(os.Stdin)
		hexPk, err = r.ReadBytes('\n')
		if err == io.EOF {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	hexPk = bytes.TrimSpace(hexPk)
	_, err = hex.Decode(pk[:], hexPk)
	zeroBytes(hexPk)
	return err
}

func privKeyFromHex(hexPk string, pk *[32]byte) error {
	hexTrimmed := strings.TrimSpace(hexPk)
	_, err := hex.Decode(pk[:], []byte(hexTrimmed))
	return err
}

func loadPrivKey(cfg *config, pk *[32]byte) error {
	if cfg.PrivKeyFile != "" {
		return decryptPrivKeyFile(cfg.PrivKeyFile, pk)
	}

	if cfg.PrivKey == "-" {
		return privKeyFromStdIn(pk)
	}

	return privKeyFromHex(cfg.PrivKey, pk)
}

func zeroBytes(s []byte) {
	for i := range s {
		s[i] = 0
	}
}

type payout struct {
	address dcrutil.Address
	amount  int64
}

func payoutsFromCSV(cfg *config) ([]*payout, error) {
	var payouts []*payout
	f, err := os.Open(cfg.CSV)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	for i := 0; ; i++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(record) != 2 {
			return nil, fmt.Errorf("record %d does not have 2 elements (%d)",
				i, len(record))
		}

		// Decode address.
		addr, err := dcrutil.DecodeAddress(record[0], cfg.chainParams)
		if err != nil {
			return nil, fmt.Errorf("record %d[0] is not an address: %v",
				i, err)
		}

		amt, err := strconv.ParseInt(record[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("record %d[1] is not an amount: %v",
				i, err)
		}

		payouts = append(payouts, &payout{
			address: addr,
			amount:  amt,
		})
	}

	return payouts, nil
}

func payoutsFromCfg(cfg *config) ([]*payout, error) {
	payouts := make([]*payout, 0, len(cfg.Addresses))

	for i, encodedAddr := range cfg.Addresses {
		amt := cfg.Amounts[i]

		// Decode address.
		addr, err := dcrutil.DecodeAddress(encodedAddr, cfg.chainParams)
		if err != nil {
			return nil, err
		}

		payouts = append(payouts, &payout{
			address: addr,
			amount:  amt,
		})
	}
	return payouts, nil
}

func loadPayouts(cfg *config) ([]*payout, error) {
	if cfg.CSV != "" {
		return payoutsFromCSV(cfg)
	}

	return payoutsFromCfg(cfg)
}

func loadOpReturnScript(cfg *config) ([]byte, error) {
	var err error
	randPayload := make([]byte, chainhash.HashSize)
	if cfg.OpReturnData != "" {
		_, err = hex.Decode(randPayload, []byte(cfg.OpReturnData))
	} else {
		_, err = rand.Read(randPayload)
	}
	if err != nil {
		return nil, err
	}
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_RETURN)
	builder.AddData(randPayload)
	return builder.Script()
}

func loadExpiry(cfg *config, c *rpcclient.Client, ctx context.Context) (uint32, error) {
	// Use the specified expiry if provided.
	if cfg.Expiry != 0 {
		return uint32(cfg.Expiry), nil
	}

	// Otherwise, find one based on the current block (either the specified
	// one or one from a dcrd instance).
	currentHeight := int64(cfg.CurrentHeight)
	if currentHeight == 0 {
		bestHash, bestHeight, err := c.GetBestBlock(ctx)
		if err != nil {
			return 0, err
		}
		log.Debugf("Best block: Height %d Hash %s", bestHeight, bestHash)

		currentHeight = int64(bestHeight)
	}

	tvi := cfg.chainParams.TreasuryVoteInterval
	mul := cfg.chainParams.TreasuryVoteIntervalMultiplier

	nextHeight := currentHeight + 1
	log.Infof("Next block height: %d", nextHeight)

	// If the current block is too close to the next TVI (which would imply
	// the start of the vote is too close to the current block) advance
	// into the next TVI to ease operational concerns about the moving the
	// data and signed transaction across air-gapped computers, posting on
	// Politeia for review and distributing across the node network, etc.
	//
	// We arbitrarily consider the height "too close" if it's less than 1/4
	// of the way to reach the TVI.
	blocksToTVI := int64(tvi) - (nextHeight % int64(tvi))
	tooCloseThresh := int64(tvi / 4)
	if blocksToTVI < tooCloseThresh {
		nextHeight = nextHeight + blocksToTVI
		log.Infof("Next block height too close to next TVI (%d blocks "+
			"to TVI; thresh=%d). Using %d as next height.",
			blocksToTVI, tooCloseThresh, nextHeight)
	}

	expiry := blockchain.CalculateTSpendExpiry(int64(nextHeight), tvi, mul)
	return expiry, nil
}

func genTspend(cfg *config, ctx context.Context) error {
	chainParams := cfg.chainParams
	relayFee := dcrutil.Amount(cfg.FeeRate)

	var c *rpcclient.Client
	var err error

	if cfg.needsDcrd() {
		c, err = rpcclient.New(cfg.dcrdConnConfig(), nil)
	}
	if err != nil {
		return err
	}

	// Figure out the expiry.
	expiry, err := loadExpiry(cfg, c, ctx)
	if err != nil {
		return err
	}

	// Figure out the OP_RETURN script.
	opretScript, err := loadOpReturnScript(cfg)
	if err != nil {
		return err
	}

	// Load the payouts.
	var totalPayout int64
	payouts, err := loadPayouts(cfg)
	if err != nil {
		return err
	}

	// Start building the TSpend Tx.
	msgTx := wire.NewMsgTx()
	msgTx.Version = wire.TxVersionTreasury
	msgTx.Expiry = expiry
	msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

	// Generate OP_TGENs outputs and calculate totals.
	for _, payout := range payouts {
		totalPayout += payout.amount

		// Create OP_TGEN prefixed script.
		p2ahs, err := txscript.PayToAddrScript(payout.address)
		if err != nil {
			return fmt.Errorf("Error generating script for addr %s: %v",
				payout.address.Address(), err)
		}
		script := make([]byte, len(p2ahs)+1)
		script[0] = txscript.OP_TGEN
		copy(script[1:], p2ahs)

		txOut := wire.NewTxOut(payout.amount, script)
		if err := CheckOutput(txOut, relayFee); err != nil {
			log.Warnf("Output %s (%d atoms) failed check: %v",
				payout.address.Address(), payout.amount, err)
		}

		// Add to transaction.
		msgTx.AddTxOut(txOut)
	}

	// Add the base TxIn.
	msgTx.AddTxIn(&wire.TxIn{
		// Stakebase transactions have no inputs, so previous outpoint
		// is zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         0, // Will calculate after fee estimate
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: []byte{}, // Empty for now
	})

	// Estimate the size. It's the size of the tx so far + the signature of
	// a TSPend which also has a fixed size.
	estimatedSize := msgTx.SerializeSize() + tspend_sigscript_size

	// Calculate fee. Inputs are <signature> <compressed key> OP_TSPEND.
	fee := FeeForSerializeSize(relayFee, estimatedSize)

	// Fill in the value in with the fee.
	msgTx.TxIn[0].ValueIn = totalPayout + int64(fee)

	// Load the priv key.
	var privKeyBytes [32]byte
	if err := loadPrivKey(cfg, &privKeyBytes); err != nil {
		return err
	}

	// Calculate TSpend signature without SigHashType. Zero out the
	// privKeyBytes afterwards as they won't be needed anymore.
	sigscript, err := txscript.TSpendSignatureScript(msgTx, privKeyBytes[:])
	zeroBytes(privKeyBytes[:])
	if err != nil {
		return err
	}
	msgTx.TxIn[0].SignatureScript = sigscript

	_, pubKeyBytes, err := stake.CheckTSpend(msgTx)
	if err != nil {
		return fmt.Errorf("CheckTSPend failed: %v", err)
	}

	// Determine the corresponding public key for debug reasons.
	var foundPiKey bool
	for i := 0; i < len(chainParams.PiKeys) && !foundPiKey; i++ {
		foundPiKey = foundPiKey || bytes.Equal(pubKeyBytes, chainParams.PiKeys[i])
	}

	// Publish the tx if requested.
	if cfg.Publish {
		_, err := c.SendRawTransaction(ctx, msgTx, true)
		if err != nil {
			return fmt.Errorf("Failed to publish tspend: %v", err)
		}
	}

	// Write the raw tx.
	rawTx, err := msgTx.Bytes()
	if err != nil {
		return err
	}

	if cfg.Out != "" {
		f, err := os.Create(cfg.Out)
		if err != nil {
			return fmt.Errorf("error creating output file: %v", err)
		}
		fmt.Fprintf(f, "%x\n", rawTx)
		f.Close()
	} else {
		fmt.Printf("%x\n", rawTx)
	}

	// Debug stuff.
	debugf := func(format string, args ...interface{}) {
		log.Infof(format, args...)
	}

	if cfg.Spew {
		debugf("%s", spew.Sdump(msgTx))
	}

	tvi := chainParams.TreasuryVoteInterval
	mul := chainParams.TreasuryVoteIntervalMultiplier
	start, _ := blockchain.CalculateTSpendWindowStart(expiry, tvi, mul)
	end, _ := blockchain.CalculateTSpendWindowEnd(expiry, tvi)

	debugf("TSpend Hash: %s", msgTx.TxHash())
	debugf("TSpend PubKey: %x", pubKeyBytes)
	debugf("Expiry: %d", expiry)
	debugf("Voting interval: %d - %d", start, end)
	debugf("Total output amount: %s", dcrutil.Amount(totalPayout))
	debugf("Total tx size: %d bytes", estimatedSize)
	debugf("Total fees: %s", dcrutil.Amount(fee))

	if !foundPiKey {
		log.Warnf("Private key does not correspond to a public Pi Key " +
			"for the specified chain")
	}

	return nil
}
