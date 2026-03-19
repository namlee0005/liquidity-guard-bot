package errors

import (
	"errors"
	"fmt"
)

// ErrorCode classifies errors for structured handling and logging.
type ErrorCode string

const (
	ErrCodeDB          ErrorCode = "DB_ERROR"
	ErrCodeExchange    ErrorCode = "EXCHANGE_ERROR"
	ErrCodeRisk        ErrorCode = "RISK_VIOLATION"
	ErrCodeConfig      ErrorCode = "CONFIG_INVALID"
	ErrCodeInternal    ErrorCode = "INTERNAL_ERROR"
	ErrCodeNotFound    ErrorCode = "NOT_FOUND"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
)

// MASError is the base error type for the Liquidity Guard Bot.
// All subsystem errors wrap this type so callers can use errors.As().
type MASError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *MASError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *MASError) Unwrap() error { return e.Cause }

// New creates a MASError without a cause.
func New(code ErrorCode, msg string) *MASError {
	return &MASError{Code: code, Message: msg}
}

// Wrap creates a MASError wrapping an underlying error.
func Wrap(code ErrorCode, msg string, cause error) *MASError {
	return &MASError{Code: code, Message: msg, Cause: cause}
}

// Subsystem error constructors.

func DBError(msg string, cause error) *MASError {
	return Wrap(ErrCodeDB, msg, cause)
}

func ExchangeError(msg string, cause error) *MASError {
	return Wrap(ErrCodeExchange, msg, cause)
}

func RiskViolation(msg string) *MASError {
	return New(ErrCodeRisk, msg)
}

func ConfigInvalid(msg string) *MASError {
	return New(ErrCodeConfig, msg)
}

func NotFound(resource string) *MASError {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found", resource))
}

// Is enables errors.Is() comparisons by ErrorCode.
func Is(err error, code ErrorCode) bool {
	var masErr *MASError
	if errors.As(err, &masErr) {
		return masErr.Code == code
	}
	return false
}
