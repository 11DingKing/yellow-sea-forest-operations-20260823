package domain

import (
	"errors"
	"fmt"
)

var (
	ErrValidation        = errors.New("validation failed")
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrForbidden         = errors.New("forbidden")
	ErrUnauthenticated   = errors.New("unauthenticated")
	ErrUnavailable       = errors.New("temporarily unavailable")
	ErrInvalidTransition = errors.New("invalid transition")
	ErrLeaseLost         = errors.New("worker lease lost")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e FieldError) Unwrap() error { return ErrValidation }

type ConflictError struct {
	Resource string
	Reason   string
}

func (e ConflictError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("%s conflicts with current state", e.Resource)
	}
	return fmt.Sprintf("%s: %s", e.Resource, e.Reason)
}

func (e ConflictError) Unwrap() error { return ErrConflict }

type TransitionError struct {
	Entity string
	From   string
	To     string
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("%s cannot transition from %s to %s", e.Entity, e.From, e.To)
}

func (e TransitionError) Unwrap() error { return ErrInvalidTransition }

type VersionConflictError struct {
	Entity   string
	Expected int64
	Actual   int64
}

func (e VersionConflictError) Error() string {
	return fmt.Sprintf("%s version conflict: expected %d, got %d", e.Entity, e.Expected, e.Actual)
}

func (e VersionConflictError) Unwrap() error { return ErrConflict }

func WrapUnavailable(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w: %v", operation, ErrUnavailable, err)
}
