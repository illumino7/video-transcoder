package main

import "net/http"

func (app *application) internalServerError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("internal server error:", "path", r.URL.Path, "method", r.Method, "err", err.Error())
	WriteJSONError(w, http.StatusInternalServerError, "Internal server error")
}

func (app *application) badRequestError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("bad request error:", "path", r.URL.Path, "method", r.Method, "err", err.Error())
	WriteJSONError(w, http.StatusBadRequest, err.Error())
}
