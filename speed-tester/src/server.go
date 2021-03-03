package main

import (
	"html/template"
	"log"
	"net/http"
	"time"
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

func (sh *SpeedTestResultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Request URL: %s", r.URL)
	log.Printf("Site root %s", sh.SiteRoot+"/")
	if r.URL.Path != sh.SiteRoot+"/" {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))

		return
	}

	if r.Method == http.MethodPost {
		go sh.Tester.runTestNow()
		http.Redirect(w, r, sh.SiteRoot+"/", http.StatusFound)
	} else {
		sh.Template.Execute(w, SpeedTestResultResponseData{
			Results: sh.Tester.Store.GetResults(),
			SiteInfo: &SiteInfo{
				SiteRoot:         sh.SiteRoot,
				SharedAssetsSite: sh.SharedAssetsSite,
			},
		})

	}

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
		"formatDate": FormatDate,
	}).ParseFiles("./src/templates/index.html"))

	handler := &SpeedTestResultHandler{
		Tester:           tester,
		Template:         t,
		SiteRoot:         siteRoot,
		SharedAssetsSite: sharedAssets,
	}

	fs := http.FileServer(http.Dir("./src/static"))

	http.Handle(siteRoot+"/static/", http.StripPrefix(siteRoot+"/static/", fs))
	http.Handle(siteRoot+"/", handler)
	log.Fatal(http.ListenAndServe(":10000", nil))
}
