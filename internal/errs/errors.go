package errs

import (
	"errors"
	"fmt"
	"time"
)

const (
	CodeUserError      = "user_error"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeReviewerFailed = "reviewer_failed"
	CodeInternalError  = "internal_error"
)

type Error struct {
	Code    string
	Message string
	Details map[string]any
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(code string, message string, err error, details map[string]any) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
		Details: cloneDetails(details),
	}
}

type Info struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
	At      string         `json:"at,omitempty"`
}

func InfoFrom(err error) *Info {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		details := cloneDetails(e.Details)
		if e.Err != nil {
			details["cause"] = e.Err.Error()
		}
		return &Info{
			Code:    e.Code,
			Message: e.Message,
			Details: details,
		}
	}
	return &Info{
		Code:    CodeInternalError,
		Message: "internal error",
		Details: map[string]any{"cause": err.Error()},
	}
}

func WithTime(info *Info, t time.Time) *Info {
	if info == nil {
		return nil
	}
	out := *info
	out.Details = cloneDetails(info.Details)
	out.At = t.UTC().Format(time.RFC3339)
	return &out
}

func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func cloneDetails(details map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range details {
		out[key] = value
	}
	return out
}
