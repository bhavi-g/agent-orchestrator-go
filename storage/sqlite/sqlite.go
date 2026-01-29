package sqlite

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	Path string
	DB   *sql.DB
}

func New(path string) *SQLiteRepository {
	return &SQLiteRepository{
		Path: path,
	}
}

func (r *SQLiteRepository) Open() error {
	db, err := sql.Open("sqlite", r.Path)
	if err != nil {
		return err
	}

	// Force connection to verify DB and create file
	if err := db.Ping(); err != nil {
		return err
	}

	r.DB = db
	return nil
}


func (r *SQLiteRepository) Close() error {
	if r.DB == nil {
		return nil
	}
	return r.DB.Close()
}
