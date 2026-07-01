package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"postdare-go/backend/internal/config"
	"postdare-go/backend/internal/migration"
)

func main() {
	dir := flag.String("dir", "./migrations", "directory containing SQL migrations")
	flag.Parse()

	cfg, err := config.Load(os.Getenv("POSTDARE_GO_CONFIG"))
	if err != nil {
		exitf("load config: %v", err)
	}
	database, err := sql.Open("mysql", cfg.Database.DSN)
	if err != nil {
		exitf("open mysql: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		exitf("ping mysql: %v", err)
	}
	if err := migration.Run(ctx, database, *dir, os.Stdout); err != nil {
		exitf("%v", err)
	}
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
