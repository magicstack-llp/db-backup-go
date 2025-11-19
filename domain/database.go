package domain

// Database represents a database entity
type Database struct {
	Name string
}

// NewDatabase creates a new Database instance
func NewDatabase(name string) *Database {
	return &Database{
		Name: name,
	}
}

