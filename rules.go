// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

// DefaultRelayFeePerKb is the default minimum relay fee policy for a mempool.
const DefaultRelayFeePerKb dcrutil.Amount = 1e4

// IsDustAmount determines whether a transaction output value and script length would
// cause the output to be considered dust.  Transactions with dust outputs are
// not standard and are rejected by mempools with default policies.
func IsDustAmount(amount dcrutil.Amount, scriptSize int, relayFeePerKb dcrutil.Amount) bool {
	// Calculate the total (estimated) cost to the network.  This is
	// calculated using the serialize size of the output plus the serial
	// size of a transaction input which redeems it.  The output is assumed
	// to be compressed P2PKH as this is the most common script type.  Use
	// the average size of a compressed P2PKH redeem input (165) rather than
	// the largest possible (txsizes.RedeemP2PKHInputSize).
	totalSize := 8 + 2 + wire.VarIntSerializeSize(uint64(scriptSize)) +
		scriptSize + 165

	// Dust is defined as an output value where the total cost to the network
	// (output size + input size) is greater than 1/3 of the relay fee.
	return int64(amount)*1000/(3*int64(totalSize)) < int64(relayFeePerKb)
}

// IsDustOutput determines whether a transaction output is considered dust.
// Transactions with dust outputs are not standard and are rejected by mempools
// with default policies.
func IsDustOutput(output *wire.TxOut, relayFeePerKb dcrutil.Amount) bool {
	// Unspendable outputs which solely carry data are not checked for dust.
	if stdscript.DetermineScriptType(output.Version, output.PkScript) == stdscript.STNullData {
		return false
	}

	// All other unspendable outputs are considered dust.
	if txscript.IsUnspendable(output.Value, output.PkScript) {
		return true
	}

	return IsDustAmount(dcrutil.Amount(output.Value), len(output.PkScript),
		relayFeePerKb)
}

// CheckOutput performs simple consensus and policy tests on a transaction
// output.  Returns with errors.Invalid if output violates consensus rules, and
// errors.Policy if the output violates a non-consensus policy.
func CheckOutput(output *wire.TxOut, relayFeePerKb dcrutil.Amount) error {
	if output.Value < 0 {
		return errors.New("transaction output amount is negative")
	}
	if output.Value > dcrutil.MaxAmount {
		return errors.New("transaction output amount exceeds maximum value")
	}
	if IsDustOutput(output, relayFeePerKb) {
		return errors.New("transaction output is dust")
	}
	return nil
}

// FeeForSerializeSize calculates the required fee for a transaction of some
// arbitrary size given a mempool's relay fee policy.
func FeeForSerializeSize(relayFeePerKb dcrutil.Amount, txSerializeSize int) dcrutil.Amount {
	fee := relayFeePerKb * dcrutil.Amount(txSerializeSize) / 1000

	if fee == 0 && relayFeePerKb > 0 {
		fee = relayFeePerKb
	}

	if fee < 0 || fee > dcrutil.MaxAmount {
		fee = dcrutil.MaxAmount
	}

	return fee
}

func sumOutputValues(outputs []*wire.TxOut) (totalOutput dcrutil.Amount) {
	for _, txOut := range outputs {
		totalOutput += dcrutil.Amount(txOut.Value)
	}
	return totalOutput
}
