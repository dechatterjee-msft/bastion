package config

import (
	"os"
	"strconv"
	"time"
)

type Options struct {
	BackupRoot      string
	MaxRetries      int
	NumberOfWorkers int
	GcRetain        time.Duration
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func getIntEnv(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}
