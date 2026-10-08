package persistence

import (
	"database/sql"
)

func Set(db *sql.DB, key, value string) error {
	_, e := db.Exec(`INSERT INTO configuration VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return e
}
func Get(db *sql.DB, key string) string {
	var v string
	_ = db.QueryRow(`SELECT value FROM configuration WHERE key=?`, key).Scan(&v)
	return v
}
