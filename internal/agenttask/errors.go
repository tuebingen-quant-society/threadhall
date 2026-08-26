package agenttask

import "errors"

var (
	ErrInvalidInput    = errors.New("invalid agent request")
	ErrForbidden       = errors.New("agent administration forbidden")
	ErrUnauthenticated = errors.New("agent authentication failed")
	ErrNotFound        = errors.New("agent resource not found")
	ErrConflict        = errors.New("agent task conflict")
	ErrBusy            = errors.New("agent persistence is busy")
)
