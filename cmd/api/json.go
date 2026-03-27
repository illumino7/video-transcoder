package main

import (
	"encoding/json"
	"net/http"
)

// WriteJSON is a standardized helper to marshal native Go structs and send them
// out as a well-formed JSON HTTP response with the correct Content-Type.

func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// WriteJSONError wraps a simple error message in our standard error envelope format
// before marshaling it to JSON.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	type envelope struct {
		Error string `json:"error"`
	}
	WriteJSON(w, status, envelope{Error: message})
}

// ReadJSON strictly parses an incoming request body into the provided destination object.
// It actively caps the payload size at 1MB to prevent out-of-memory DDoS vectors and
// disallows any JSON fields not explicitly defined in the struct.
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := 1_048_578
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(dst)
	return err
}
