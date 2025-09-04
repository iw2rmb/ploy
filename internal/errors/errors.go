package errors

import (
    "errors"
    "fmt"
    "net/http"
)

// Code is a stable machine-readable error code.
type Code string

const (
    CodeBadRequest   Code = "bad_request"
    CodeUnauthorized Code = "unauthorized"
    CodeForbidden    Code = "forbidden"
    CodeNotFound     Code = "not_found"
    CodeConflict     Code = "conflict"
    CodeValidation   Code = "validation_error"
    CodeInternal     Code = "internal_error"
)

// Error is a typed application error with HTTP mapping and optional details.
type Error struct {
    Code       Code        `json:"code"`
    Message    string      `json:"message"`
    Details    interface{} `json:"details,omitempty"`
    HTTPStatus int         `json:"-"`
    cause      error       `json:"-"`
}

func (e *Error) Error() string {
    if e == nil {
        return "<nil>"
    }
    if e.cause != nil {
        return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// Match allows errors.Is to work with wrapped errors of the same Code.
func (e *Error) Is(target error) bool {
    t, ok := target.(*Error)
    if !ok {
        return false
    }
    return e.Code == t.Code
}

// New constructs a new typed error.
func New(code Code, httpStatus int, msg string, details interface{}, cause error) *Error {
    return &Error{Code: code, HTTPStatus: httpStatus, Message: msg, Details: details, cause: cause}
}

// Helper constructors
func BadRequest(msg string, details interface{}) *Error   { return New(CodeBadRequest, http.StatusBadRequest, msg, details, nil) }
func Unauthorized(msg string, details interface{}) *Error { return New(CodeUnauthorized, http.StatusUnauthorized, msg, details, nil) }
func Forbidden(msg string, details interface{}) *Error    { return New(CodeForbidden, http.StatusForbidden, msg, details, nil) }
func NotFound(msg string, details interface{}) *Error     { return New(CodeNotFound, http.StatusNotFound, msg, details, nil) }
func Conflict(msg string, details interface{}) *Error     { return New(CodeConflict, http.StatusConflict, msg, details, nil) }
func Validation(msg string, details interface{}) *Error   { return New(CodeValidation, http.StatusUnprocessableEntity, msg, details, nil) }
func Internal(msg string, cause error) *Error             { return New(CodeInternal, http.StatusInternalServerError, msg, nil, cause) }

// Wrap maps an arbitrary error to an internal error with message and optional details.
func Wrap(err error, msg string, details interface{}) *Error {
    if err == nil {
        return nil
    }
    var te *Error
    if errors.As(err, &te) {
        // already typed, preserve
        return te
    }
    return Internal(msg, err)
}

// From maps any error to a typed error (Internal if not already typed).
func From(err error) *Error {
    if err == nil {
        return nil
    }
    var te *Error
    if errors.As(err, &te) {
        return te
    }
    return Internal("internal error", err)
}

// Validation helpers
func ValidateNotEmpty(field, value string) *Error {
    if value == "" {
        return Validation(fmt.Sprintf("%s is required", field), map[string]string{"field": field})
    }
    return nil
}

