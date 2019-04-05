package logger

import (
	log "github.com/sirupsen/logrus"
)

func LogSetup(lvl string) {
	var level log.Level
	var err error
	if lvl == "" {
		level = log.InfoLevel
	} else {
		level, err = log.ParseLevel(lvl)
		if err != nil {
			log.Info("Log Level is not setup right, falling back to info level")
			level = log.InfoLevel
		}
	}
	log.SetLevel(level)
}
