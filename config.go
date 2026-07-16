package main

import "os"

const (
	defaultListenAddress = ":8080"
	defaultDatabasePath  = "var/sqlite.db"
)

type config struct {
	listenAddress string
	databasePath  string
}

func configFromEnvironment() config {
	return config{
		listenAddress: valueOrDefault("HTMXCHAT_LISTEN_ADDRESS", defaultListenAddress),
		databasePath:  valueOrDefault("HTMXCHAT_DATABASE_PATH", defaultDatabasePath),
	}
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
