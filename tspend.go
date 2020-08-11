package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
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

	// Figure out the expiry if not commanded to use a specific one.
	expiry := uint32(cfg.Expiry)
	if expiry == 0 {
		bestHash, bestHeight, err := c.GetBestBlock(ctx)
		if err != nil {
			return err
		}
		log.Debugf("Best block: Height %d Hash %s", bestHeight, bestHash)

		expiry = blockchain.CalculateTSpendExpiry(int64(bestHeight+1),
			chainParams.TreasuryVoteInterval,
			chainParams.TreasuryVoteIntervalMultiplier)
	}

	// Figure out the OP_RETURN script.
	randPayload := make([]byte, chainhash.HashSize)
	if cfg.OpReturnData != "" {
		_, err = hex.Decode(randPayload, []byte(cfg.OpReturnData))
	} else {
		_, err = rand.Read(randPayload)
	}
	if err != nil {
		return err
	}

	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_RETURN)
	builder.AddData(randPayload)
	opretScript, err := builder.Script()
	if err != nil {
		return err
	}

	// Start building the TSpend Tx.
	msgTx := wire.NewMsgTx()
	msgTx.Version = wire.TxVersionTreasury
	msgTx.Expiry = expiry
	msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

	// Generate OP_TGENs outputs and calculate totals.
	var totalPayout int64
	for i, encodedAddr := range cfg.Addresses {
		amt := cfg.Amounts[i]

		// While looping calculate total amount
		totalPayout += amt

		// Decode address.
		addr, err := dcrutil.DecodeAddress(encodedAddr, chainParams)
		if err != nil {
			return err
		}

		// Create OP_TGEN prefixed script.
		p2ahs, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return fmt.Errorf("Error generating script for addr %s: %v",
				encodedAddr, err)
		}
		script := make([]byte, len(p2ahs)+1)
		script[0] = txscript.OP_TGEN
		copy(script[1:], p2ahs)

		txOut := wire.NewTxOut(int64(amt), script)
		if err := CheckOutput(txOut, relayFee); err != nil {
			log.Warnf("Output %s (%d atoms) failed check: %v",
				encodedAddr, amt, err)
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

	rawTx, err := msgTx.Bytes()
	if err != nil {
		return err
	}
	fmt.Printf("Raw TSPend:\n%x\n\n", rawTx)
	if cfg.Spew {
		spew.Dump(msgTx)
		fmt.Println("")
	}
	fmt.Printf("TSpend Hash: %s\n", msgTx.TxHash())
	fmt.Printf("TSpend PubKey: %x\n", pubKeyBytes)
	fmt.Printf("Expiry: %d\n", expiry)
	fmt.Printf("Total output amount: %s\n", dcrutil.Amount(totalPayout))
	fmt.Printf("Total tx size: %d bytes\n", estimatedSize)
	fmt.Printf("Total fees: %s\n", dcrutil.Amount(fee))
	fmt.Println("")

	if !foundPiKey {
		log.Warnf("Private key does not correspond to a public Pi Key " +
			"for the specified chain")
	}

	return nil
}
