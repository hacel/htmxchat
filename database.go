package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hacel/htmxchat/templates"
	_ "github.com/mattn/go-sqlite3"
)

const databaseSchema = `
	CREATE TABLE IF NOT EXISTS chats (
		author  TEXT NOT NULL,
		content TEXT NOT NULL,
		time    DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS chats_time_idx ON chats (time);
`

func openDatabase(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path must not be empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := path + "?_busy_timeout=5000&_journal_mode=WAL"
	if path == ":memory:" {
		dsn = ":memory:"
	}
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if _, err := database.Exec(databaseSchema); err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return database, nil
}

func recentMessages(ctx context.Context, database *sql.DB, limit int) ([]templates.Message, error) {
	if limit <= 0 {
		return nil, errors.New("message limit must be positive")
	}
	rows, err := database.QueryContext(ctx, `
		SELECT author, content, time
		FROM (
			SELECT author, content, time
			FROM chats
			ORDER BY time DESC
			LIMIT ?
		)
		ORDER BY time ASC
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}
	defer rows.Close()

	messages := make([]templates.Message, 0, limit)
	for rows.Next() {
		var message templates.Message
		if err := rows.Scan(&message.Author, &message.Content, &message.Time); err != nil {
			return nil, fmt.Errorf("scan recent message: %w", err)
		}
		message.Color = colorFor(message.Author)
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recent messages: %w", err)
	}
	return messages, nil
}

func saveMessage(ctx context.Context, database *sql.DB, message templates.Message) error {
	_, err := database.ExecContext(
		ctx,
		`INSERT INTO chats (author, content, time) VALUES (?, ?, ?)`,
		message.Author,
		message.Content,
		message.Time,
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}
