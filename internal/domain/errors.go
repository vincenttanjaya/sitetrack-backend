package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("operation conflicts with current state")
)

func Invalid(message string) error  { return fmt.Errorf("%w: %s", ErrInvalid, message) }
func Conflict(message string) error { return fmt.Errorf("%w: %s", ErrConflict, message) }
