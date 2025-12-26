package utils

import (
	"github.com/google/uuid"
)

// GenerateUUIDv7 generates a new UUIDv7 string.
// UUIDv7 is time-ordered, making it suitable for database indexing
// and providing natural chronological sorting.
func GenerateUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// IsValidUUID checks if a string is a valid UUID format.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
