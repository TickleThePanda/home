package main

import (
	"html/template"
	"log"
	"net/http"
)

type SpeedTestResultHandler struct {
	Store    *SpeedTestResultStore
	Template *template.Template
	SiteRoot string
}

type SiteInfo struct {
	SiteRoot string
}

type SpeedTestResultResponseData struct {
	Results  *SpeedTestResults
	SiteInfo *SiteInfo
}

func (sh *SpeedTestResultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request URL: %s", r.URL)
	if r.URL.Path != sh.SiteRoot || r.URL.Path != sh.SiteRoot+"/" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))

		return
	}
	sh.Template.Execute(w, SpeedTestResultResponseData{
		Results: sh.Store.GetAll(),
		SiteInfo: &SiteInfo{
			SiteRoot: sh.SiteRoot,
		},
	})
}

func handleRequests(store *SpeedTestResultStore, siteRoot string) {
	t := template.New("index")
	t, _ = template.ParseFiles("./src/templates/index.html")

	handler := &SpeedTestResultHandler{
		Store:    store,
		Template: t,
		SiteRoot: siteRoot,
	}

	fs := http.FileServer(http.Dir("./src/static"))

	http.Handle(siteRoot+"/static/", http.StripPrefix(siteRoot+"/static/", fs))
	http.Handle(siteRoot+"/", handler)
	log.Fatal(http.ListenAndServe(":10000", nil))
}
