// Copyright 2019 Andrew 'Diddymus' Rolfe. All rights reserved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

// Package config provides access to all of the tunable settings of a WolfMUD
// server. The server's default configuration values may be overridden using a
// configuration file. The name of the default configuration file is config.wrj
// in the server's data path. By default the server's data path is '../data',
// the data directory in the parent's path.
//
// An alternative data path and/or configuration file can be specified using
// the WOLFMUD_DIR environment variable. The format is:
//
//   WOLFMUD_DIR=<data path><configuration file>
//
// The default value, if not specified, is equivalent to:
//
//   WOLFMUD_DIR=../data/config.wrj
//
// If only the data path is specified the default configuration file will be
// used. If only a configuration file is specified the default data path will
// be used. The data path may be absolute or relative to the current path.
//
// NOTE: The server's data path is used to locate zone files and player files
// as well as the configuration file.
//
// Alternative data directories and/or configuration files can be useful for
// testing environments or for running multiple server instances.
package config

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.wolfmud.org/WolfMUD.git/recordjar"
	"code.wolfmud.org/WolfMUD.git/recordjar/decode"
	"code.wolfmud.org/WolfMUD.git/text"
)

// Server default configuration
var Server = struct {
	Host           string        // Host for server to listen on
	Port           string        // Port for server to listen on
	Greeting       []byte        // Connection greeting
	IdleTimeout    time.Duration // Idle connection disconnect time
	MaxPlayers     int           // Max number of players allowed to login at once
	LogClient      bool          // Log connecting IP address and port of client?
	DataDir        string        // Main data directory
	SetPermissions bool          // Set permissions on created account files?
}{
	Host:           "127.0.0.1",
	Port:           "4001",
	Greeting:       []byte(""),
	IdleTimeout:    10 * time.Minute,
	MaxPlayers:     1024,
	DataDir:        ".",
	SetPermissions: false,
}

// Per IP connection quota default configuration
var Quota = struct {
	Window  time.Duration // Period quota records connections for
	Timeout time.Duration // Period after which quota are reset
	Stats   time.Duration // Minimum reporting period for quota stats
}{
	Window:  0,
	Timeout: 0,
	Stats:   0,
}

// Stats default configuration
var Stats = struct {
	Rate time.Duration // Stats collection and display rate
	GC   bool          // Run garbage collection before stat collection
}{
	Rate: 10 * time.Second,
	GC:   false,
}

// Inventory default configuration
var Inventory = struct {
	CrowdSize int // If inventory has more player than this it's a crowd
}{
	CrowdSize: 10,
}

// Login default configuration
var Login = struct {
	AccountLength  int
	PasswordLength int
	SaltLength     int
}{
	AccountLength:  10,
	PasswordLength: 10,
	SaltLength:     32,
}

// Debugging configuration
var Debug = struct {
	LongLog    bool // Long log with microseconds & filename?
	Panic      bool // Let goroutines panic and stop server?
	AllowDump  bool // Allow use of #DUMP/#UDUMP/#LDUMP commands?
	AllowDebug bool // Allow use of #DEBUG command?
	Events     bool // Log events? - this can make the log quite noisy
	Things     bool // Log additional information for Thing?
}{
	LongLog:    false,
	Panic:      false,
	AllowDump:  false,
	AllowDebug: false,
	Events:     false,
	Things:     false,
}

// Load reads the configuration file and overrides the default configuration
// values with any values found.
func init() {

	// Setup global logging format
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds | log.LUTC)
	log.Printf("Server started, logging using UTC timezone.")

	// Seed default random source
	rand.Seed(time.Now().UnixNano())

	f, err := openConfig()

	if err != nil {
		log.Printf("Configuration file error: %s", err)
		return
	}

	if f == nil {
		log.Print("No configuration file used. Using defaults.")
		return
	}

	Server.DataDir = filepath.Dir(f.Name())
	log.Printf("Loading: %s", f.Name())

	j := recordjar.Read(f, "server.greeting")
	f.Close()
	record := j[0]

	// NOTE: a recordjar will uppercase all fieldnames so we need to use
	// uppercase switch cases below.
	for field, data := range record {
		switch field {

		// Main server settings
		case "SERVER.HOST":
			Server.Host = decode.String(data)
		case "SERVER.PORT":
			Server.Port = decode.String(data)
		case "SERVER.IDLETIMEOUT":
			Server.IdleTimeout = decode.Duration(data)
		case "SERVER.MAXPLAYERS":
			Server.MaxPlayers = decode.Integer(data)
		case "SERVER.LOGCLIENT":
			Server.LogClient = decode.Boolean(data)
		case "SERVER.GREETING":
			Server.Greeting = text.Colorize(text.Unfold(decode.Bytes(data)))

		// Per IP connection quotas
		case "QUOTA.WINDOW":
			Quota.Window = decode.Duration(data)
		case "QUOTA.TIMEOUT":
			Quota.Timeout = decode.Duration(data)
		case "QUOTA.STATS":
			Quota.Stats = decode.Duration(data)

		// Stats settings
		case "STATS.RATE":
			Stats.Rate = decode.Duration(data)
		case "STATS.GC":
			Stats.GC = decode.Boolean(data)

		// Inventory settings
		case "INVENTORY.CROWDSIZE":
			Inventory.CrowdSize = decode.Integer(data)

		// Login settings
		case "LOGIN.ACCOUNTLENGTH":
			Login.AccountLength = decode.Integer(data)
		case "LOGIN.PASSWORDLENGTH":
			Login.PasswordLength = decode.Integer(data)
		case "LOGIN.SALTLENGTH":
			Login.SaltLength = decode.Integer(data)

		// Debug settings
		case "DEBUG.LONGLOG":
			Debug.LongLog = decode.Boolean(data)
		case "DEBUG.PANIC":
			Debug.Panic = decode.Boolean(data)
		case "DEBUG.ALLOWDUMP":
			Debug.AllowDump = decode.Boolean(data)
		case "DEBUG.ALLOWDEBUG":
			Debug.AllowDebug = decode.Boolean(data)
		case "DEBUG.EVENTS":
			Debug.Events = decode.Boolean(data)
		case "DEBUG.THINGS":
			Debug.Things = decode.Boolean(data)

		// Unknow setting
		default:
			log.Printf("Unknown setting %s: %s", field, data)
		}
	}

	log.Printf("Data Path: %s", Server.DataDir)

	Server.SetPermissions, err = filesystemCheck(Server.DataDir)
	log.Printf("Set permissions on player account files: %t", Server.SetPermissions)
	if err != nil {
		log.Printf("Error checking permissions, %s", err)
	}

	switch {
	case Quota.Window == 0:
		log.Printf("IP connection quotas are disabled.")
	case Quota.Timeout != 0:
		log.Printf(
			"Per IP connection quota is 4 in %s, reset after %s.",
			Quota.Window, Quota.Timeout,
		)
	case Quota.Timeout == 0:
		log.Printf(
			"Per IP connection quota is 4 in %s, reset after no connections for %s.",
			Quota.Window, Quota.Window,
		)
	}

	switch {
	case Quota.Window == 0:
	case Quota.Stats == 0:
		log.Printf("Quota statistics logging disabled.")
	default:
		log.Printf("Minimum quota statistics logging period is %s.", Quota.Stats)
	}

	if !Debug.LongLog {
		log.SetFlags(log.LstdFlags | log.LUTC)
		log.Printf("Switching to short log format.")
	}
}

// openConfig tries to locate and open the configuration file to use. See
// package comments for details of the default server data path and
// configuration file used.
func openConfig() (config *os.File, err error) {

	// Has user supplied path ± specific file?
	env := os.Getenv("WOLFMUD_DIR")
	if env != "" {
		log.Printf("Found environment variable WOLFMUD_DIR=%s", env)
	}
	dir, file := filepath.Split(env)

	if dir == "" && strings.ToUpper(file) == "NONE" {
		return nil, nil
	}

	// Is the file actually a directory without a final separator?
	if file != "" && filepath.Ext(file) != ".wrj" {
		dir = filepath.Join(dir, file)
		file = ""
	}

	// If no user supplied path use the data directory in the parent of the
	// current working directory.
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return nil, err
		}
		dir = filepath.Join(dir, "..", "data")
	}

	// If no configuration file provided use the default
	if file == "" {
		file = "config.wrj"
	}

	// Make sure path + file is good
	path := filepath.Join(dir, file)
	if path, err = filepath.Abs(path); err != nil {
		return nil, err
	}

	// Try and open configuration file
	if config, err = os.Open(path); err != nil {
		return nil, err
	}

	log.Printf("Found configuration file: %s", path)
	return config, nil
}

// filesystemCheck tests to see if the filesystem the passed path is on
// supports the changing of file permissions. If they do true will be returned,
// otherwise false. The returned error will be non-nil if an error occurs when
// checking.
func filesystemCheck(path string) (bool, error) {

	p := filepath.Join(path, "check.tmp")

	defer os.Remove(p)

	var (
		f    *os.File
		info os.FileInfo
		err  error
	)

	if err = os.Remove(p); err != nil {
		if !os.IsNotExist(err) {
			return false, err
		}
	}

	if f, err = os.Create(p); err != nil {
		return false, err
	}

	if err = f.Chmod(0660); err != nil {
		return false, err
	}

	if info, err = os.Stat(p); err != nil {
		return false, err
	}

	if err = f.Close(); err != nil {
		return false, err
	}

	if info.Mode()&os.FileMode(0777) != os.FileMode(0660) {
		return false, fmt.Errorf("Cannot set permissions to 0660: 0%o", info.Mode())
	}

	return true, nil
}
