package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 10 * time.Second

func run(ctx context.Context, cfg config) error {
	database, err := openDatabase(cfg.databasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	chat := newChatServer(database)
	e := newHTTPServer(chat)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- e.Start(cfg.listenAddress)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		chat.closeAll()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, configFromEnvironment()); err != nil {
		log.Fatal(err)
	}
}
