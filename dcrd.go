package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/rpcclient/v7"
)

const (
	// The following define the minium json rpc server the underlying dcrd
	// instance should be running on. These are interpreted according to
	// semver, so any difference in major versions causes an error while we
	// accept any minor version greater than or equal to the minimum.
	wantJsonRpcMajor uint32 = 7
	wantJsonRpcMinor uint32 = 0
)

var (
	errBlockNotFound       = errors.New("block not found")
	errBlockHeightAfterTip = errors.New("block height after tip")
)

// checkDcrd verifies whether the specified dcrd instance fulfills the required
// elements for running the dcrros server.
//
// This method returns the version string reported by the dcrd instance.
func checkDcrd(ctx context.Context, c *rpcclient.Client,
	chain *chaincfg.Params, ignoreRpcVersionErr bool) (string, error) {

	info, err := c.GetBlockChainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get blockchain info from dcrd: %v", err)
	}

	if info.Chain != chain.Name {
		return "", fmt.Errorf("dcrros and dcrd network mismatch (want %s, "+
			"got %s)", chain.Name, info.Chain)
	}

	version, err := c.Version(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to query dcrd vesion: %v", err)
	}
	rpcVersion, ok := version["dcrdjsonrpcapi"]
	if !ok {
		return "", fmt.Errorf("dcrd did not provide the 'dcrdjsonrpcapi' version")
	}
	if rpcVersion.Major != wantJsonRpcMajor || rpcVersion.Minor < wantJsonRpcMinor {
		err := fmt.Errorf("dcrd running on unsupported rpcjson version "+
			"(want %d.%d got %s)", wantJsonRpcMajor,
			wantJsonRpcMinor, rpcVersion.VersionString)
		if !ignoreRpcVersionErr {
			return "", err
		}

		log.Warnf("%v - ignoring error as comanded", err)
	}

	dcrdVersion, ok := version["dcrd"]
	if !ok {
		return "", fmt.Errorf("dcrd did not provide the 'dcrd' version")
	}
	return dcrdVersion.VersionString, nil
}

// CheckDcrd verifies whether the dcrd in the given address is reachable and
// usable by a Server instance.
//
// Note that while we do some perfunctory tests on the specified dcrd instance,
// there's no guarantee the underlying server won't change (e.g. changing the
// chain after a restart) so this is only offered as a helper for early testing
// during process startup for easier error reporting.
func CheckDcrd(ctx context.Context, connCfg *rpcclient.ConnConfig,
	chainParams *chaincfg.Params, ignoreRpcVersionErr bool) error {

	// We make a copy of the passed config because we change some of the
	// parameters locally to ensure they are configured as needed by the
	// Server struct.
	connCfg.DisableConnectOnNew = true
	connCfg.DisableAutoReconnect = true
	connCfg.HTTPPostMode = false
	c, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return err
	}

	err = c.Connect(ctx, false)
	if err != nil {
		return err
	}

	_, err = checkDcrd(ctx, c, chainParams, ignoreRpcVersionErr)
	c.Disconnect()
	return err
}
