package platform

import (
	"fmt"
)

// APIError describes an HTTP error returned by the NEWO platform.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Temporary reports whether the error may succeed on retry.
func (e *APIError) Temporary() bool {
	if e == nil {
		return false
	}
	if e.Status == 429 {
		return true
	}
	return e.Status >= 500 && e.Status < 600
}
