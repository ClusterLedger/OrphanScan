package main

import (
	"io"
	"os"

	"golang.org/x/term"
)

// commandRuntime captures process state once at the command boundary. Tests can
// supply independent streams and environment values without changing globals.
type commandRuntime struct {
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	terminal     bool
	ci           string
	doNotTrack   bool
	configDir    string
	configDirErr error
}

func systemRuntime() commandRuntime {
	configDir, err := os.UserConfigDir()
	return commandRuntime{
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr,
		terminal: term.IsTerminal(int(os.Stdin.Fd())) &&
			term.IsTerminal(int(os.Stdout.Fd())) &&
			term.IsTerminal(int(os.Stderr.Fd())),
		ci:           os.Getenv("CI"),
		doNotTrack:   os.Getenv("DO_NOT_TRACK") == "1",
		configDir:    configDir,
		configDirErr: err,
	}
}
