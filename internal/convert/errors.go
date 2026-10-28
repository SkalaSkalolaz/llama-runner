package convert

import (
	"encoding/json"
	"net/http"
)

// WriteAnthropicError writes a well-formed Anthropic Messages API error:
//
//	{"type":"error","error":{"type":"...","message":"..."}}
//
// http.Error must not be used instead: it returns text/plain, which Claude
// Code displays as an opaque "API Error" without parsing the message.
//
// Only usable on error paths BEFORE a response body has started. Once the
// headers and the first SSE event are on the wire, the status is fixed and
// the only way to signal failure inside a stream is an `event: error` SSE
// event (see ConvertOpenAIStreamToAnthropic).
func WriteAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	WriteJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

// AnthropicErrorType maps an upstream HTTP status code to the closest
// Anthropic error type (invalid_request_error, authentication_error,
// permission_error, not_found_error, request_too_large, rate_limit_error);
// any unmapped status falls back to the generic api_error.
func AnthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	}
	return "api_error"
}

// WriteJSON sets the JSON content type, writes status, and encodes v as the writes status, and encodes v as the
// response body. Encoding errors are swallowed: headers are already sent, so
// there is nothing meaningful left to report to the client.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
