package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/rpcclient/v6"
	"github.com/decred/slog"
	"github.com/jessevdk/go-flags"
)

var appName = "tspend"

func version() string {
	return "0.1.0"
}

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
	ShowVersion bool `short:"V" long:"version" description:"Display version information and exit"`

	ConfigFile string   `short:"C" long:"configfile" description:"Path to configuration file"`
	Listeners  []string `long:"listen" description:"Add an interface/port to listen for connections (default all interfaces port: 9128, testnet: 19128, simnet: 29128)"`
	DebugLevel string   `short:"d" long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`

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

	// TSpend data

	FeeRate       int64    `long:"feerate" description:"Fee rate for the tspend in atoms/kB"`
	PrivKey       string   `long:"privkey" description:"Private key to use to sign tspend"`
	PrivKeyFile   string   `long:"privkeyfile" description:"Private key file to use to sign tspend"`
	OpReturnData  string   `long:"opreturndata" description:"OP_RETURN payload data. Random data if unspencified"`
	Publish       bool     `long:"publish" description:"Directly publish the tspend"`
	Expiry        int      `long:"expiry" description:"Expiry to use"`
	CurrentHeight int      `short:"c" long:"currentheight" description:"Current blockchain height to calculate a sane expiry from"`
	Addresses     []string `long:"address" description:"List of addresses to send to. Number of addresses must match amounts"`
	Amounts       []int64  `long:"amount" description:"List of amounts to send in atoms. Number of amounts must match addresses"`
	CSV           string   `long:"csv" description:"Generate the tspend based on a csv file"`
	Spew          bool     `long:"spew" description:"Spew the result tspend"`

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

// needsDcrd returns true if the config means the app will need to connect to
// the dcrd instance.
func (c *config) needsDcrd() bool {
	needsBestHeight := c.Expiry == 0 && c.CurrentHeight == 0
	return needsBestHeight || c.Publish
}

func (c *config) privKeyFromStdin() bool {
	return c.PrivKey == "-"
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

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	_, ok := slog.LevelFromString(logLevel)
	return ok
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsystems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) error {
	// When the specified string doesn't have any delimiters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
		// Validate debug log level.
		if !validLogLevel(debugLevel) {
			str := "the specified debug level [%v] is invalid"
			return fmt.Errorf(str, debugLevel)
		}

		// Change the logging level for all subsystems.
		setLogLevels(debugLevel)

		return nil
	}

	// Split the specified string into subsystem/level pairs while detecting
	// issues and update the log levels accordingly.
	for _, logLevelPair := range strings.Split(debugLevel, ",") {
		if !strings.Contains(logLevelPair, "=") {
			str := "the specified debug level contains an invalid " +
				"subsystem/level pair [%v]"
			return fmt.Errorf(str, logLevelPair)
		}

		// Extract the specified subsystem and log level.
		fields := strings.Split(logLevelPair, "=")
		subsysID, logLevel := fields[0], fields[1]

		// Validate subsystem.
		if _, exists := subsystemLoggers[subsysID]; !exists {
			str := "the specified subsystem [%v] is invalid -- " +
				"supported subsystems %v"
			return fmt.Errorf(str, subsysID, supportedSubsystems())
		}

		// Validate log level.
		if !validLogLevel(logLevel) {
			str := "the specified debug level [%v] is invalid"
			return fmt.Errorf(str, logLevel)
		}

		setLogLevel(subsysID, logLevel)
	}

	return nil
}

func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DcrdCertPath: defaultDcrdCertPath,
		DebugLevel:   defaultLogLevel,
		FeeRate:      int64(DefaultRelayFeePerKb),
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

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Printf("%s version %s (Go version %s %s/%s)\n",
			appName, version(),
			runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil, nil, errCmdDone
	}

	// Special show command to list supported subsystems and exit.
	if preCfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		return nil, nil, errCmdDone
	}

	// If the config file path has not been modified by user, then
	// we'll use the default config file path.
	if preCfg.ConfigFile == "" {
		preCfg.ConfigFile = defaultConfigFile
	}

	// Load additional config from file.
	var configFileError error
	parser := flags.NewParser(&cfg, flags.Default)

	err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config "+
				"file: %v\n", err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		configFileError = err
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

	// Number of addresses and amounts must match.
	if len(cfg.Addresses) != len(cfg.Amounts) {
		return nil, nil, fmt.Errorf("Number of addresses (%d) must match "+
			"number of amounts (%d)", len(cfg.Addresses), len(cfg.Amounts))
	}

	// Initialize log rotation.  After log rotation has been initialized,
	// the logger variables may be used.
	logDir := strings.Replace(defaultLogDir, string(defaultActiveNet),
		string(cfg.activeNet), 1)
	logPath := filepath.Join(logDir, appName+".log")
	initLogRotator(logPath)
	setLogLevels(defaultLogLevel)

	// Parse, validate, and set debug log level(s).
	if err := parseAndSetDebugLevels(cfg.DebugLevel); err != nil {
		err := fmt.Errorf("%s: %v", funcName, err.Error())
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Only check dcrd stuff if we'll need to connect to it.
	if cfg.needsDcrd() {
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

		// Attempt an early connection to the dcrd server and verify if it's a
		// reasonable backend for dcrros operations. We ignore the error here
		// because it's only possible due to unspecified network (which
		// shouldn't happen in this function).
		dcrdCfg := cfg.dcrdConnConfig()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = CheckDcrd(ctx, dcrdCfg, cfg.chainParams)
		if err != nil {
			return nil, nil, fmt.Errorf("error while checking underlying "+
				"dcrd: %v", err)
		}
	}

	// Fill in the default PrivKeyFile if both it and PrivKey are empty.
	if cfg.PrivKeyFile == "" && cfg.PrivKey == "" {
		cfg.PrivKeyFile = filepath.Join(defaultConfigDir, string(cfg.activeNet)+".key")
	}
	if cfg.PrivKeyFile != "" {
		if _, err := os.Stat(cfg.PrivKeyFile); err != nil {
			return nil, nil, fmt.Errorf("PrivKeyFile error: %v", err)
		}
	}

	// Warn about missing config file only after all other configuration is
	// done.  This prevents the warning on help messages and invalid
	// options.  Note this should go directly before the return.
	if configFileError != nil {
		log.Debugf("%v", configFileError)
	}

	return &cfg, remainingArgs, nil
}
