// Package errs defines the sentinel errors and the structured error type used
// by every layer of go-pdf. The public pdf package re-exports these values so
// that internal packages can return them without importing the root package.
package errs

import (
	"errors"
	"fmt"
)

var (
	ErrCorrupt     = errors.New("pdf: corrupt file")
	ErrPassword    = errors.New("pdf: password required or incorrect")
	ErrUnsupported = errors.New("pdf: unsupported feature")
	ErrBudget      = errors.New("pdf: resource budget exceeded")
	ErrClosed      = errors.New("pdf: document closed")
	ErrCanceled    = errors.New("pdf: operation canceled")
)

// Error is the structured error returned by all public operations.
//
// Err may be nil when the caller only needs the diagnostic metadata. Error()
// never includes document text, metadata values, or stream contents.
type Error struct {
	Code   string
	Op     string
	Offset int64
	Obj    int
	Page   int
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	msg := "pdf"
	if e.Op != "" {
		msg += ": " + e.Op
	}
	if e.Code != "" {
		msg += " [" + e.Code + "]"
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	if e.Obj != 0 {
		msg += fmt.Sprintf(" (obj %d)", e.Obj)
	}
	if e.Offset >= 0 && e.Code != "" {
		msg += fmt.Sprintf(" (offset %d)", e.Offset)
	}
	if e.Page >= 0 && e.Code != "" {
		msg += fmt.Sprintf(" (page %d)", e.Page)
	}
	return msg
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Wrap returns an *Error unless err is already one. Offset and page are set to
// -1 when the caller does not know them.
func Wrap(err error, code, op string, offset int64, obj, page int) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return err
	}
	return &Error{Code: code, Op: op, Offset: offset, Obj: obj, Page: page, Err: err}
}
