package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/PorticoMediaServer/portico-server/internal/app"
	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	db, err := database.Open(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	result, err := app.BackfillArtworkRenditions(context.Background(), cfg, db)
	_ = json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
