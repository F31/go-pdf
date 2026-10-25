package pdf

import (
	"github.com/F31/go-pdf/internal/diag"
	"github.com/F31/go-pdf/internal/errs"
)

var (
	ErrCorrupt     = errs.ErrCorrupt
	ErrPassword    = errs.ErrPassword
	ErrUnsupported = errs.ErrUnsupported
	ErrBudget      = errs.ErrBudget
	ErrClosed      = errs.ErrClosed
	ErrCanceled    = errs.ErrCanceled
)

// Error is the structured error returned by public operations.
type Error = errs.Error

// Level and Diagnostic mirror the internal diagnostic model without exposing
// internal package types in the public API.
type Level int

const (
	LevelInfo     Level = Level(diag.Info)
	LevelWarn     Level = Level(diag.Warn)
	LevelDegraded Level = Level(diag.Degraded)
	LevelError    Level = Level(diag.Error)
)

func (l Level) String() string { return diag.Level(l).String() }

type Diagnostic struct {
	Level  Level  `json:"level"`
	Code   string `json:"code"`
	Msg    string `json:"msg"`
	Page   int    `json:"page,omitempty"`
	Obj    int    `json:"obj,omitempty"`
	Offset int64  `json:"offset,omitempty"`
}
