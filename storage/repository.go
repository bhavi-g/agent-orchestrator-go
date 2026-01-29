package storage

type Repository interface {
	Open() error
	Close() error
}
