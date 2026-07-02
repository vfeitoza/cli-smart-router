package infrastructure

import (
	"encoding/json"
)

// Envelope is the JSON RPC envelope expected by CLIProxyAPI plugins.
type Envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *EnvelopeError  `json:"error,omitempty"`
}

// EnvelopeError describes a plugin RPC error.
type EnvelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OKEnvelope wraps a successful plugin RPC result.
func OKEnvelope(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{OK: true, Result: raw})
}

// ErrorEnvelope wraps a plugin RPC error.
func ErrorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(Envelope{OK: false, Error: &EnvelopeError{Code: code, Message: message}})
	return raw
}
