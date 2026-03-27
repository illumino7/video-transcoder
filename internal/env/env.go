// Package env provides strongly-typed utility wrappers for retrieving environment
// variables with safe fallback defaults, preventing boilerplate parsing logic throughout the app.
package env

import (
	"os"
	"strconv"
	"time"
)

// GetString abstracts reading an environment variable, returning a guaranteed fallback string if absent.
func GetString(key, fallback string) string {
	key, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return key
}

func GetInt(key string, fallback int) int {
	key, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	i, err := strconv.Atoi(key)
	if err != nil {
		return fallback
	}
	return i
}

func GetBool(key string, fallback bool) bool {
	key, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(key)
	if err != nil {
		return fallback
	}
	return b
}

func GetTime(key string, fallback time.Duration) time.Duration {
	key, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	timeKey, err := time.ParseDuration(key)
	if err != nil {
		return fallback
	}
	return timeKey
}
