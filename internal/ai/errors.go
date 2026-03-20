package ai

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/openai/openai-go"
)

// AIError wraps an upstream API error with a classified Kind for differentiated handling.
type AIError struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

type ErrorKind int

const (
	ErrTransient  ErrorKind = iota // retriable server/network errors (5xx, 408, 409)
	ErrRateLimit                   // 429 Too Many Requests
	ErrAuth                        // 401 Unauthorized / 403 Forbidden
	ErrBadRequest                  // 400 Bad Request / validation / content policy
	ErrUnknown                     // anything else
)

func (k ErrorKind) String() string {
	switch k {
	case ErrTransient:
		return "transient"
	case ErrRateLimit:
		return "rate_limit"
	case ErrAuth:
		return "auth"
	case ErrBadRequest:
		return "bad_request"
	default:
		return "unknown"
	}
}

func (e *AIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *AIError) Unwrap() error { return e.Cause }

func IsRateLimit(err error) bool  { return hasKind(err, ErrRateLimit) }
func IsAuth(err error) bool       { return hasKind(err, ErrAuth) }
func IsTransient(err error) bool  { return hasKind(err, ErrTransient) }
func IsBadRequest(err error) bool { return hasKind(err, ErrBadRequest) }

func hasKind(err error, kind ErrorKind) bool {
	var ae *AIError
	return errors.As(err, &ae) && ae.Kind == kind
}

// classifyError inspects an error returned by the openai-go SDK and wraps it
// as an *AIError with the appropriate kind.
func classifyError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return &AIError{Kind: ErrTransient, Message: "network or connection error", Cause: err}
	}

	switch {
	case apiErr.StatusCode == http.StatusTooManyRequests:
		return &AIError{Kind: ErrRateLimit, Message: "rate limit exceeded", Cause: apiErr}
	case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
		return &AIError{Kind: ErrAuth, Message: "authentication failed", Cause: apiErr}
	case apiErr.StatusCode == http.StatusBadRequest:
		return &AIError{Kind: ErrBadRequest, Message: apiErr.Message, Cause: apiErr}
	case apiErr.StatusCode >= http.StatusInternalServerError,
		apiErr.StatusCode == http.StatusRequestTimeout,
		apiErr.StatusCode == http.StatusConflict:
		return &AIError{Kind: ErrTransient, Message: "server error", Cause: apiErr}
	default:
		return &AIError{Kind: ErrUnknown, Message: apiErr.Message, Cause: apiErr}
	}
}
