package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

type Level int

const (
	LevelError Level = iota
	LevelInfo
)

var (
	currentLevel = LevelInfo
	mu           sync.Mutex
)

// SetLevel sets the global log level.
func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = l
}

// Setup initializes the standard logger output.
func Setup(w io.Writer) {
	log.SetOutput(w)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

// Info logs informative messages if the level allows.
func Info(format string, v ...interface{}) {
	if currentLevel >= LevelInfo {
		output("INFO: "+format, v...)
	}
}

// Error logs error messages.
func Error(format string, v ...interface{}) {
	if currentLevel >= LevelError {
		output("ERROR: "+format, v...)
	}
}

// Fatal logs independent of error level and exits.
func Fatal(format string, v ...interface{}) {
	output("FATAL: "+format, v...)
	os.Exit(1)
}

func output(format string, v ...interface{}) {
	// Calldepth 3 to skip this function, Info/Error, and get to caller
	log.Output(3, fmt.Sprintf(format, v...))
}
