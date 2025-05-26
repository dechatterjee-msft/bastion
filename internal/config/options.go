package config

import (
	"os"
	"strconv"
)

type Options struct {
	BackupRoot string
	MaxRetries int
}

func LoadFromEnv() *Options {
	return &Options{
		BackupRoot: getEnv("BACKUP_ROOT", "/backups"),
		MaxRetries: getIntEnv("MAX_RETRIES", 3),
	}
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
