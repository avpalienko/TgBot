package ai

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/openai/openai-go"
)

func makeAPIError(statusCode int, message string) *openai.Error {
	return &openai.Error{
		StatusCode: statusCode,
		Message:    message,
		Request:    &http.Request{Method: "POST", URL: &url.URL{Path: "/v1/responses"}},
		Response:   &http.Response{StatusCode: statusCode},
	}
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantKind ErrorKind
	}{
		{"nil returns nil", nil, 0},
		{"429 rate limit", makeAPIError(429, "rate limit"), ErrRateLimit},
		{"401 unauthorized", makeAPIError(401, "invalid api key"), ErrAuth},
		{"403 forbidden", makeAPIError(403, "forbidden"), ErrAuth},
		{"400 bad request", makeAPIError(400, "invalid prompt"), ErrBadRequest},
		{"500 server error", makeAPIError(500, "internal error"), ErrTransient},
		{"502 bad gateway", makeAPIError(502, "bad gateway"), ErrTransient},
		{"503 service unavailable", makeAPIError(503, "unavailable"), ErrTransient},
		{"408 request timeout", makeAPIError(408, "timeout"), ErrTransient},
		{"409 conflict", makeAPIError(409, "conflict"), ErrTransient},
		{"422 unprocessable", makeAPIError(422, "unprocessable"), ErrUnknown},
		{"non-API error", fmt.Errorf("connection refused"), ErrTransient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := classifyError(tt.err)
			if tt.err == nil {
				if result != nil {
					t.Fatalf("expected nil, got %v", result)
				}
				return
			}
			var ae *AIError
			if !errors.As(result, &ae) {
				t.Fatalf("expected *AIError, got %T", result)
			}
			if ae.Kind != tt.wantKind {
				t.Fatalf("expected kind %v, got %v", tt.wantKind, ae.Kind)
			}
		})
	}
}

func TestErrorKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind ErrorKind
		want string
	}{
		{ErrTransient, "transient"},
		{ErrRateLimit, "rate_limit"},
		{ErrAuth, "auth"},
		{ErrBadRequest, "bad_request"},
		{ErrUnknown, "unknown"},
		{ErrorKind(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := tt.kind.String(); got != tt.want {
				t.Fatalf("ErrorKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestAIErrorUnwrap(t *testing.T) {
	t.Parallel()

	cause := fmt.Errorf("root cause")
	ae := &AIError{Kind: ErrTransient, Message: "test", Cause: cause}

	if !errors.Is(ae, cause) {
		t.Fatalf("Unwrap should return the original cause")
	}
}

func TestAIErrorMessage(t *testing.T) {
	t.Parallel()

	t.Run("with cause", func(t *testing.T) {
		t.Parallel()
		ae := &AIError{Kind: ErrRateLimit, Message: "rate limit exceeded", Cause: fmt.Errorf("429")}
		got := ae.Error()
		if got != "rate_limit: rate limit exceeded: 429" {
			t.Fatalf("unexpected error string: %q", got)
		}
	})

	t.Run("without cause", func(t *testing.T) {
		t.Parallel()
		ae := &AIError{Kind: ErrAuth, Message: "authentication failed"}
		got := ae.Error()
		if got != "auth: authentication failed" {
			t.Fatalf("unexpected error string: %q", got)
		}
	})
}

func TestPredicates(t *testing.T) {
	t.Parallel()

	rl := &AIError{Kind: ErrRateLimit, Message: "test"}
	auth := &AIError{Kind: ErrAuth, Message: "test"}
	transient := &AIError{Kind: ErrTransient, Message: "test"}
	badReq := &AIError{Kind: ErrBadRequest, Message: "test"}
	plain := fmt.Errorf("not an AI error")

	if !IsRateLimit(rl) {
		t.Fatalf("IsRateLimit should return true for ErrRateLimit")
	}
	if IsRateLimit(auth) {
		t.Fatalf("IsRateLimit should return false for ErrAuth")
	}
	if !IsAuth(auth) {
		t.Fatalf("IsAuth should return true for ErrAuth")
	}
	if !IsTransient(transient) {
		t.Fatalf("IsTransient should return true for ErrTransient")
	}
	if !IsBadRequest(badReq) {
		t.Fatalf("IsBadRequest should return true for ErrBadRequest")
	}
	if IsRateLimit(plain) {
		t.Fatalf("IsRateLimit should return false for non-AIError")
	}

	wrapped := fmt.Errorf("wrapped: %w", rl)
	if !IsRateLimit(wrapped) {
		t.Fatalf("IsRateLimit should work through error wrapping")
	}
}
