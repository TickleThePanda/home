package main

import (
	"html/template"
	"log"
	"net/http"
)

type SpeedTestResultHandler struct {
	Store    *SpeedTestResultStore
	Template *template.Template
}

func (sh *SpeedTestResultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sh.Template.Execute(w, sh.Store.GetAll())
}

func handleRequests(store *SpeedTestResultStore) {
	t := template.New("index")
	t, _ = template.ParseFiles("./src/templates/index.html")

	handler := &SpeedTestResultHandler{
		Store:    store,
		Template: t,
	}

	fs := http.FileServer(http.Dir("./src/static"))

	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.Handle("/", handler)
	log.Fatal(http.ListenAndServe(":10000", nil))
}
