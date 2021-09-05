package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

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

	sh.Template.Execute(w, SpeedTestResultResponseData{
		Results: sh.Tester.Store.GetResults(),
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
	}).ParseFiles("./src/templates/index.html"))

	handler := &SpeedTestResultHandler{
		Tester:           tester,
		Template:         t,
		SiteRoot:         siteRoot,
		SharedAssetsSite: sharedAssets,
	}

	r := mux.NewRouter()

	fs := http.FileServer(http.Dir("./src/static"))

	r.PathPrefix(siteRoot + "/static/").Handler(http.StripPrefix(siteRoot+"/static/", fs))

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

	srv := &http.Server{
		Handler:      r,
		Addr:         ":10000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
