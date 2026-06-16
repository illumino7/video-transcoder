package main

import "net/http"

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}
