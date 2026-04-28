package config

import "os"

type Config struct {
	DatabaseURL   string
	SteamAPIKey   string
	LeetifyAPIKey string
	Port          string
}

func Load() Config {
	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SteamAPIKey:   os.Getenv("STEAM_API_KEY"),
		LeetifyAPIKey: os.Getenv("LEETIFY_API_KEY"),
		Port:          getEnvOrDefault("PORT", "8080"),
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
