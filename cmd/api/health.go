package main

import "net/http"

// health is a simple readiness probe to verify the API server is responsive
// and securely available to load balancers or container orchestrators.

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}
