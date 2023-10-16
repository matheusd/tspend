package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/rpcclient/v8"
	"github.com/jessevdk/go-flags"
)

var (
	defaultDcrdDir      = dcrutil.AppDataDir("dcrd", false)
	defaultDcrdCertPath = filepath.Join(defaultDcrdDir, "rpc.cert")

	errCmdDone = errors.New("command is done")
)

type config struct {
	// Network

	MainNet bool `long:"mainnet" description:"Use the main network"`
	TestNet bool `long:"testnet" description:"Use the test network"`
	SimNet  bool `long:"simnet" description:"Use the simulation test network"`

	// Dcrd Connection Options

	DcrdConnect   string `short:"s" long:"dcrdconnect" description:"Network address of the RPC interface of the dcrd node to connect to (default: localhost port 9109, testnet: 19109, simnet: 19556)"`
	DcrdCertPath  string `short:"c" long:"dcrdcertpath" description:"File path location of the dcrd RPC certificate"`
	DcrdCertBytes string `long:"dcrdcertbytes" description:"The pem-encoded RPC certificate for dcrd"`
	DcrdUser      string `short:"u" long:"dcrduser" description:"RPC username to authenticate with dcrd"`
	DcrdPass      string `short:"P" long:"dcrdpass" description:"RPC password to authenticate with dcrd"`

	Height uint32 `long:"height" description:"Perform estimate for the specified height instead of tip"`

	// The rest of the members of this struct are filled by loadConfig().

	// activeNet   chainNetwork
	chainParams *chaincfg.Params
}

func (c *config) dcrdConnConfig() *rpcclient.ConnConfig {
	return &rpcclient.ConnConfig{
		Host:         c.DcrdConnect,
		Endpoint:     "ws",
		User:         c.DcrdUser,
		Pass:         c.DcrdPass,
		Certificates: []byte(c.DcrdCertBytes),
	}
}

func loadConfig() (*config, error) {
	// Default config.
	cfg := config{
		DcrdCertPath: defaultDcrdCertPath,
	}

	preParser := flags.NewParser(&cfg, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, errCmdDone
		}
	}

	switch {
	case (cfg.MainNet && !(cfg.TestNet || cfg.SimNet)):
		fallthrough
	case !(cfg.TestNet || cfg.SimNet):
		cfg.chainParams = chaincfg.MainNetParams()
	case cfg.TestNet && !(cfg.MainNet || cfg.SimNet):
		cfg.chainParams = chaincfg.TestNet3Params()
	case cfg.SimNet && !(cfg.MainNet || cfg.TestNet):
		cfg.chainParams = chaincfg.SimNetParams()
	default:
		return nil, fmt.Errorf("invalid network config - only " +
			"one of --mainnet, --testnet and --simnet may " +
			"be specified")
	}

	if cfg.DcrdConnect == "" {
		switch {
		case cfg.chainParams.Name == "mainnet":
			cfg.DcrdConnect = "localhost:9109"
		case cfg.chainParams.Name == "testnet3":
			cfg.DcrdConnect = "localhost:19109"
		case cfg.chainParams.Name == "simnet":
			cfg.DcrdConnect = "localhost:19556"
		}
	}

	// Load the appropriate dcrd rpc.cert file.
	if len(cfg.DcrdCertBytes) == 0 && cfg.DcrdCertPath != "" {
		f, err := os.ReadFile(cfg.DcrdCertPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load dcrd cert "+
				"file: %v", err)
		}
		cfg.DcrdCertBytes = string(f)
	}

	return &cfg, nil
}
