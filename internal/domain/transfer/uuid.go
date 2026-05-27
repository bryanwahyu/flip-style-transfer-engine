package transfer

import "github.com/google/uuid"

func newUUID() string { return uuid.New().String() }

func validateUUID(s string) error {
	_, err := uuid.Parse(s)
	return err
}
