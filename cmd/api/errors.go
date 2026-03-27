package main

import "net/http"

// internalServerError logs the fault internally for debugging and emits a safe,
// generic 500 response to the client to avoid leaking sensitive stack traces.

func (app *application) internalServerError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("internal server error:", "path", r.URL.Path, "method", r.Method, "err", err.Error())
	WriteJSONError(w, http.StatusInternalServerError, "Internal server error")
}

// badRequestError tracks a client-side error and reflects it back with a 400 response
// structure so the frontend can display actionable feedback.
func (app *application) badRequestError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("bad request error:", "path", r.URL.Path, "method", r.Method, "err", err.Error())
	WriteJSONError(w, http.StatusBadRequest, err.Error())
}
