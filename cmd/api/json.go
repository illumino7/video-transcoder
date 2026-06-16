package main

import (
	"encoding/json"
	"net/http"
)

// WriteJSON marshals and sends a JSON HTTP response.
func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// WriteJSONError wraps and sends an error message as JSON.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	type envelope struct {
		Error string `json:"error"`
	}
	WriteJSON(w, status, envelope{Error: message})
}

// ReadJSON parses a JSON request body into dst, enforcing a 1MB limit.
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := 1_048_578
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(dst)
	return err
}
