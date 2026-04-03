package auth

import (
	"log"

	"github.com/jmoiron/sqlx"
)

// SeedDefaultUser inserts the admin user if it does not exist.
func SeedDefaultUser(db *sqlx.DB) error {
	_, err := db.Exec(`
		INSERT INTO users (id, username, email, password_hash, role, created_at, updated_at)
		VALUES (gen_random_uuid(), 'admin', 'admin@agenthub.com',
			'$2a$10$N9qo8uLOickgx2ZMRZoMy.Mr/Ct7dJ8P0N9XqO9JxA0dJ9XqO9J',
			'admin', NOW(), NOW())
		ON CONFLICT (username) DO NOTHING`)
	if err != nil {
		log.Printf("[auth] seed user error: %v", err)
		return err
	}
	log.Println("[auth] default admin user seeded")
	return nil
}
