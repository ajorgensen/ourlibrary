package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/ajorgensen/ourlibrary/internal/account"
	"github.com/ajorgensen/ourlibrary/internal/config"
	"github.com/ajorgensen/ourlibrary/internal/database"
	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(context.Background(), os.Args[1:]); err != nil {
		logger.Error("user command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 2 || args[0] != "add" {
		return errors.New("usage: user add <email>")
	}

	email, err := account.NormalizeEmail(args[1])
	if err != nil {
		return fmt.Errorf("email: %w", err)
	}

	pool, err := database.Open(ctx, config.Load().DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	user, err := store.New(pool).CreateUser(ctx, email)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return fmt.Errorf("user %q already exists", email)
		}
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("created user %s (id %s)\n", user.Email, user.ID)
	return nil
}
