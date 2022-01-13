package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/rpcclient/v7"
	"github.com/jessevdk/go-flags"
)

const appName = "voteprogress"

type chainNetwork string

const (
	cnMainNet chainNetwork = "mainnet"
	cnTestNet chainNetwork = "testnet"
	cnSimNet  chainNetwork = "simnet"
)

// defaultDcrdCfg returns the default rpc connect address for the given
// network.
func (c chainNetwork) defaultDcrdRPCConnect() string {
	switch c {
	case cnMainNet:
		return "localhost:9109"
	case cnTestNet:
		return "localhost:19109"
	case cnSimNet:
		return "localhost:19556"
	default:
		panic("unknown chainNetwork")
	}
}

func (c chainNetwork) chainParams() *chaincfg.Params {
	switch c {
	case cnMainNet:
		return chaincfg.MainNetParams()
	case cnTestNet:
		return chaincfg.TestNet3Params()
	case cnSimNet:
		return chaincfg.SimNetParams()
	default:
		panic("unknown chainNetwork")
	}
}

const (
	defaultLogLevel  = "info"
	defaultActiveNet = cnMainNet
)

var (
	defaultConfigFilename = appName + ".conf"
	defaultConfigDir      = dcrutil.AppDataDir(appName, false)
	defaultLogDir         = filepath.Join(defaultConfigDir, "logs", string(defaultActiveNet))
	defaultConfigFile     = filepath.Join(defaultConfigDir, defaultConfigFilename)
	defaultDcrdDir        = dcrutil.AppDataDir("dcrd", false)
	defaultDcrdCertPath   = filepath.Join(defaultDcrdDir, "rpc.cert")

	errCmdDone = errors.New("cmd is done while parsing config options")
)

type config struct {
	ConfigFile string `short:"C" long:"configfile" description:"Path to configuration file"`

	// Network

	MainNet bool `long:"mainnet" description:"Use the main network"`
	TestNet bool `long:"testnet" description:"Use the test network"`
	SimNet  bool `long:"simnet" description:"Use the simulation test network"`

	// Dcrd Connection Options

	DcrdConnect   string `long:"dcrdconnect" description:"Network address of the RPC interface of the dcrd node to connect to (default: localhost port 9109, testnet: 19109, simnet: 19556)"`
	DcrdCertPath  string `long:"dcrdcertpath" description:"File path location of the dcrd RPC certificate"`
	DcrdCertBytes string `long:"dcrdcertbytes" description:"The pem-encoded RPC certificate for dcrd"`
	DcrdUser      string `short:"u" long:"dcrduser" description:"RPC username to authenticate with dcrd"`
	DcrdPass      string `short:"P" long:"dcrdpass" description:"RPC password to authenticate with dcrd"`

	// The rest of the members of this struct are filled by loadConfig().

	activeNet   chainNetwork
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

func (c *config) fillActiveNet() error {
	numNets := 0
	c.activeNet = defaultActiveNet
	if c.MainNet {
		numNets++
		c.activeNet = cnMainNet
	}
	if c.TestNet {
		numNets++
		c.activeNet = cnTestNet
	}
	if c.SimNet {
		numNets++
		c.activeNet = cnSimNet
	}
	if numNets > 1 {
		return errors.New("mainnet, testnet and simnet params can't be " +
			"used together -- choose one of the three")
	}

	c.chainParams = c.activeNet.chainParams()
	return nil
}

func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DcrdCertPath: defaultDcrdCertPath,
	}

	// Pre-parse the command line options to see if an alternative config
	// file was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg
	preParser := flags.NewParser(&preCfg, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, errCmdDone
		}
	}

	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)

	// If the config file path has not been modified by user, then
	// we'll use the default config file path.
	if preCfg.ConfigFile == "" {
		preCfg.ConfigFile = defaultConfigFile
	}

	// Load additional config from file.
	parser := flags.NewParser(&cfg, flags.Default)

	err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config "+
				"file: %v\n", err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, nil, err
	}

	// Create the home directory if it doesn't already exist.
	funcName := "loadConfig"
	err = os.MkdirAll(defaultConfigDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		str := "%s: Failed to create home directory: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Determine the final network.
	if err := cfg.fillActiveNet(); err != nil {
		return nil, nil, err
	}

	// Determine the default dcrd connect address based on the
	// selected network.
	if cfg.DcrdConnect == "" {
		cfg.DcrdConnect = cfg.activeNet.defaultDcrdRPCConnect()
	}

	// Load the appropriate dcrd rpc.cert file.
	if len(cfg.DcrdCertBytes) == 0 && cfg.DcrdCertPath != "" {
		f, err := ioutil.ReadFile(cfg.DcrdCertPath)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to load dcrd cert "+
				"file: %v", err)
		}
		cfg.DcrdCertBytes = string(f)
	}

	return &cfg, remainingArgs, nil
}
