package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port         int
	StorageRoot  string
	WorkerCount  int
	MediaPath    string
	WhisperModel string
	EnableGPU    bool
}

func Load() Config {
	return Config{
		Port:         getEnvInt("VE_PORT", 8090),
		StorageRoot:  getEnv("VE_STORAGE_ROOT", "./data"),
		WorkerCount:  getEnvInt("VE_WORKER_COUNT", 2),
		MediaPath:    getEnv("VE_MEDIA_PATH", "./test-media"),
		WhisperModel: getEnv("VE_WHISPER_MODEL", "base"),
		EnableGPU:    getEnv("VE_ENABLE_GPU", "auto") != "false",
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
