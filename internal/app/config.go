package app

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	DBPath      string
	ListenAddr  string
	HTTPTimeout time.Duration
	UserAgent   string
	HubToken    string
}

func LoadConfig() Config {
	return Config{
		DBPath:      env("DB_PATH", "/data/pulls.sqlite"),
		ListenAddr:  env("LISTEN_ADDR", ":8080"),
		HTTPTimeout: envDur("HTTP_TIMEOUT", 15*time.Second),
		UserAgent:   env("USER_AGENT", "dockerhub-pull-watcher/1.0"),
		HubToken:    strings.TrimSpace(os.Getenv("DOCKERHUB_TOKEN")),
	}
}

func env(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func envDur(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
