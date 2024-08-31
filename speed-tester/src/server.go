package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

//go:embed templates/*
var templatesFs embed.FS

//go:embed static/*
var staticFs embed.FS

type SpeedTestResultHandler struct {
	Tester           *SpeedTester
	Template         *template.Template
	SiteRoot         string
	SharedAssetsSite string
}

type SiteInfo struct {
	SiteRoot         string
	SharedAssetsSite string
}

type SpeedTestResultResponseData struct {
	Results  *SpeedTestResults
	SiteInfo *SiteInfo
}

func (sh *SpeedTestResultHandler) Index(w http.ResponseWriter, r *http.Request) {

	log.Printf("URL: %v", r.URL)

	results := sh.Tester.Store.GetResults()

	log.Printf("Results: %v", results.Entries)

	sh.Template.Execute(w, SpeedTestResultResponseData{
		Results: results,
		SiteInfo: &SiteInfo{
			SiteRoot:         sh.SiteRoot,
			SharedAssetsSite: sh.SharedAssetsSite,
		},
	})

}

func (sh *SpeedTestResultHandler) TestNow(w http.ResponseWriter, r *http.Request) {
	go sh.Tester.runTestNow()
	http.Redirect(w, r, sh.SiteRoot+"/", http.StatusFound)
}

func (sh *SpeedTestResultHandler) Export(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/csv")
	sh.Tester.Store.Export(w)
}

func (sh *SpeedTestResultHandler) GetLastMonth(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sh.Tester.Store.GetResults().LastMonth())
}

func (sh *SpeedTestResultHandler) GetLastYear(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sh.Tester.Store.GetResults().LastYear())
}

func (sh *SpeedTestResultHandler) Delete(w http.ResponseWriter, r *http.Request) {
	error := r.ParseForm()
	if error != nil {
		http.Error(w, "Failed to parse form", http.StatusInternalServerError)
	}

	toDeleteTimestamp := r.FormValue("to_delete_timestamp")
	go sh.Tester.Store.Delete(toDeleteTimestamp)
	http.Redirect(w, r, sh.SiteRoot+"/", http.StatusFound)

}

func FormatDate8601(t time.Time) string {
	return t.Format(time.RFC3339)
}

func FormatDate(t time.Time) string {
	suffix := "th"
	switch t.Day() {
	case 1, 21, 31:
		suffix = "st"
	case 2, 22:
		suffix = "nd"
	case 3, 23:
		suffix = "rd"
	}
	return t.Format("Mon 2" + suffix + " Jan - 15:04")
}

func handleRequests(tester *SpeedTester, siteRoot string, sharedAssets string) {
	t := template.Must(template.New("index.html").Funcs(template.FuncMap{
		"formatDate":     FormatDate,
		"formatDate8601": FormatDate8601,
	}).ParseFS(templatesFs, "templates/index.html"))

	handler := &SpeedTestResultHandler{
		Tester:           tester,
		Template:         t,
		SiteRoot:         siteRoot,
		SharedAssetsSite: sharedAssets,
	}

	r := mux.NewRouter()

	fs := http.FileServer(http.FS(staticFs))

	r.PathPrefix(siteRoot + "/static/").Handler(http.StripPrefix(siteRoot, fs))

	r.Path(siteRoot + "/").
		Methods(http.MethodGet).
		HandlerFunc(handler.Index)

	r.Path(siteRoot + "/").
		Methods(http.MethodPost).
		HandlerFunc(handler.TestNow)

	r.Path(siteRoot + "/history/lastMonth/").
		HandlerFunc(handler.GetLastMonth)

	r.Path(siteRoot + "/history/lastYear/").
		HandlerFunc(handler.GetLastYear)

	r.Path(siteRoot + "/export/").
		HandlerFunc(handler.Export)

	r.Path(siteRoot + "/history/delete/").
		Methods(http.MethodPost).
		HandlerFunc(handler.Delete)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":10000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
