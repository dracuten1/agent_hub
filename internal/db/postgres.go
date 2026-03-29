package db

import (
	"embed"
	"log"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations.sql
var migrationsFS embed.FS

func RunMigrations(db *sqlx.DB) error {
	migrations, err := migrationsFS.ReadFile("migrations.sql")
	if err != nil {
		return err
	}

	_, err = db.Exec(string(migrations))
	if err != nil {
		return err
	}

	log.Println("Migrations completed successfully")
	return nil
}
