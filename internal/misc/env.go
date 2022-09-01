package misc

import (
	"os"
	"strconv"
	"time"
)

func GetEnvStr(env, def string) string {
	if value := os.Getenv(env); value != "" {
		return value
	}
	return def
}

func GetEnvSeconds(env string, def time.Duration) time.Duration {
	if value, err := strconv.ParseFloat(os.Getenv(env), 64); err == nil {
		return time.Duration(value * float64(time.Second))
	}
	return def
}

func GetEnvBool(env string, def bool) bool {
	if value, err := strconv.ParseBool(os.Getenv(env)); err == nil {
		return value
	}
	return def
}
