//go:build !prod

package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/irbis-sh/zen-desktop/internal/constants"
	"gopkg.in/natefinch/lumberjack.v2"
)

// More on the logging setup in README.md.

func SetupLogger() error {
	logsDir, err := getLogsDir(constants.AppName)
	if err != nil {
		return fmt.Errorf("get logs directory: %w", err)
	}

	fileLogger := &lumberjack.Logger{
		Filename:   filepath.Join(logsDir, "application.log"),
		MaxSize:    5,
		MaxBackups: 5,
		MaxAge:     1,
		Compress:   true,
	}

	// Write to the file first: io.MultiWriter stops at the first writer error,
	// and os.Stdout is invalid when a GUI-subsystem build is launched without
	// a console. Ordering the file logger first guarantees application.log is
	// always written while still mirroring to the console when one exists.
	log.SetOutput(io.MultiWriter(fileLogger, os.Stdout))

	return nil
}
