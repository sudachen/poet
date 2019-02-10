// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Copyright (c) 2017-2019 The Spacemesh developers

package main

import (
	"fmt"
	"github.com/btcsuite/btcutil"
	"github.com/jessevdk/go-flags"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	defaultConfigFilename  = "poet.conf"
	defaultDataDirname     = "data"
	defaultLogLevel        = "debug"
	defaultLogDirname      = "logs"
	defaultLogFilename     = "poet.log"
	defaultMaxLogFiles     = 3
	defaultMaxLogFileSize  = 10
	defaultRPCPort         = 50002
	defaultRESTPort        = 8080
	defaultCoreServiceMode = false
	defaultN               = 3
	defaultHashFunction    = "sha256"
)

var (
	defaultPoetDir    = btcutil.AppDataDir("poet", false)
	defaultConfigFile = filepath.Join(defaultPoetDir, defaultConfigFilename)
	defaultDataDir    = filepath.Join(defaultPoetDir, defaultDataDirname)
	defaultLogDir     = filepath.Join(defaultPoetDir, defaultLogDirname)
)

type serviceConfig struct {
	N            int    `long:"n" description:"time parameter"`
	HashFunction string `long:"hashfunction" description:"hash function"`
}

type coreServiceConfig struct {
	N            int    `long:"n" description:"time parameter"`
	HashFunction string `long:"hashfunction" description:"hash function"`
}

// config defines the configuration options for poet.
//
// See loadConfig for further details regarding the
// configuration loading+parsing process.
type config struct {
	PoetDir         string `long:"poetdir" description:"The base directory that contains poet's data, logs, configuration file, etc."`
	ConfigFile      string `short:"c" long:"configfile" description:"Path to configuration file"`
	DataDir         string `short:"b" long:"datadir" description:"The directory to store poet's data within"`
	LogDir          string `long:"logdir" description:"Directory to log output."`
	MaxLogFiles     int    `long:"maxlogfiles" description:"Maximum logfiles to keep (0 for no rotation)"`
	MaxLogFileSize  int    `long:"maxlogfilesize" description:"Maximum logfile size in MB"`
	RawRPCListener  string `short:"r" long:"rpclisten" description:"The interface/port/socket to listen for RPC connections"`
	RawRESTListener string `short:"w" long:"restlisten" description:"The interface/port/socket to listen for REST connections"`
	RPCListener     net.Addr
	RESTListener    net.Addr

	LogLevel   string `short:"l" long:"loglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical}"`
	CPUProfile string `long:"cpuprofile" description:"Write CPU profile to the specified file"`
	Profile    string `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65535"`

	CoreServiceMode bool `long:"core" description:"Enable poet in core service mode"`

	CoreService *coreServiceConfig `group:"Core Service" namespace:"core"`
	Service     *serviceConfig     `group:"Service"`
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
func loadConfig() (*config, error) {
	defaultCfg := config{
		PoetDir:         defaultPoetDir,
		ConfigFile:      defaultConfigFile,
		DataDir:         defaultDataDir,
		LogLevel:        defaultLogLevel,
		LogDir:          defaultLogDir,
		MaxLogFiles:     defaultMaxLogFiles,
		MaxLogFileSize:  defaultMaxLogFileSize,
		RawRPCListener:  fmt.Sprintf("localhost:%d", defaultRPCPort),
		RawRESTListener: fmt.Sprintf("localhost:%d", defaultRESTPort),
		CoreServiceMode: defaultCoreServiceMode,
		Service: &serviceConfig{
			N:            defaultN,
			HashFunction: defaultHashFunction,
		},
		CoreService: &coreServiceConfig{
			N:            defaultN,
			HashFunction: defaultHashFunction,
		},
	}

	// Pre-parse the command line options to pick up an alternative config
	// file.
	preCfg := defaultCfg
	if _, err := flags.Parse(&preCfg); err != nil {
		return nil, err
	}

	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)

	// If the config file path has not been modified by the user, then we'll
	// use the default config file path. However, if the user has modified
	// their poetdir, then we should assume they intend to use the config
	// file within it.

	preCfg.PoetDir = cleanAndExpandPath(preCfg.PoetDir)
	preCfg.ConfigFile = cleanAndExpandPath(preCfg.ConfigFile)
	if preCfg.PoetDir != defaultPoetDir {
		if preCfg.ConfigFile == defaultConfigFile {
			preCfg.ConfigFile = filepath.Join(
				preCfg.PoetDir, defaultConfigFilename,
			)
		}
	}

	// Next, load any additional configuration options from the file.
	var configFileError error
	cfg := preCfg
	if err := flags.IniParse(preCfg.ConfigFile, &cfg); err != nil {
		// If it's a parsing related error, then we'll return
		// immediately, otherwise we can proceed as possibly the config
		// file doesn't exist which is OK.
		if _, ok := err.(*flags.IniError); ok {
			return nil, err
		}

		configFileError = err
	}

	// Finally, parse the remaining command line options again to ensure
	// they take precedence.
	if _, err := flags.Parse(&cfg); err != nil {
		return nil, err
	}

	// If the provided lnd directory is not the default, we'll modify the
	// path to all of the files and directories that will live within it.
	if cfg.PoetDir != defaultPoetDir {
		cfg.DataDir = filepath.Join(cfg.PoetDir, defaultDataDirname)
		cfg.LogDir = filepath.Join(cfg.PoetDir, defaultLogDirname)
	}

	// Create the poet directory if it doesn't already exist.
	funcName := "loadConfig"
	if err := os.MkdirAll(cfg.PoetDir, 0700); err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		str := "%s: Failed to create lnd directory: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	// As soon as we're done parsing configuration options, ensure all paths
	// to directories and files are cleaned and expanded before attempting
	// to use them later on.
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)

	// Ensure that the user didn't attempt to specify non-positive values
	// for the poet n parameter
	if cfg.Service.N < 1 || cfg.CoreService.N < 1 {
		fmt.Fprintln(os.Stderr, usageMessage)
		err := fmt.Errorf("%s: n must be positive", funcName)
		return nil, err
	}

	// Initialize logging at the default logging level.
	initLogRotator(
		filepath.Join(cfg.LogDir, defaultLogFilename),
		cfg.MaxLogFileSize, cfg.MaxLogFiles,
	)

	if !validLogLevel(cfg.LogLevel) {
		fmt.Fprintln(os.Stderr, usageMessage)
		err := fmt.Errorf("the specified log level (%v) is invalid", cfg.LogLevel)
		return nil, err
	}

	// Change the logging level for all subsystems.
	setLogLevels(cfg.LogLevel)

	// Resolve the RPC listener
	addr, err := net.ResolveTCPAddr("tcp", cfg.RawRPCListener)
	if err != nil {
		return nil, err
	}
	cfg.RPCListener = addr

	// Resolve the REST listener
	addr, err = net.ResolveTCPAddr("tcp", cfg.RawRESTListener)
	if err != nil {
		return nil, err
	}
	cfg.RESTListener = addr

	// Warn about missing config file only after all other configuration is
	// done.  This prevents the warning on help messages and invalid
	// options.  Note this should go directly before the return.
	if configFileError != nil {
		poetLog.Warnf("%v", configFileError)
	}

	return &cfg, nil
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
// This function is taken from https://github.com/btcsuite/btcd
func cleanAndExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		var homeDir string
		user, err := user.Current()
		if err == nil {
			homeDir = user.HomeDir
		} else {
			homeDir = os.Getenv("HOME")
		}

		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but the variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		fallthrough
	case "off":
		return true
	}
	return false
}