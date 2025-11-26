package main

import (
	"embed"
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

type OdinCounterHandler struct {
	Store    *OdinCounterStore
	Template *template.Template
	SiteRoot string
}

type SiteInfo struct {
	SiteRoot string
}

type OdinCounterResponseData struct {
	TodayCount  int
	DailyCounts []DailyCount
	SiteInfo    *SiteInfo
}

func (h *OdinCounterHandler) Index(w http.ResponseWriter, r *http.Request) {
	log.Printf("URL: %v", r.URL)

	todayCount, err := h.Store.GetTodayCount()
	if err != nil {
		log.Printf("Error getting today's count: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	dailyCounts, err := h.Store.GetDailyCounts()
	if err != nil {
		log.Printf("Error getting daily counts: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := OdinCounterResponseData{
		TodayCount:  todayCount,
		DailyCounts: dailyCounts,
		SiteInfo: &SiteInfo{
			SiteRoot: h.SiteRoot,
		},
	}

	if err := h.Template.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
	}
}

func (h *OdinCounterHandler) Export(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/csv")
	if err := h.Store.Export(w); err != nil {
		log.Printf("Error exporting data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handleRequests(store *OdinCounterStore, siteRoot string) {
	t := template.Must(template.New("index.html").ParseFS(templatesFs, "templates/index.html"))

	handler := &OdinCounterHandler{
		Store:    store,
		Template: t,
		SiteRoot: siteRoot,
	}

	r := mux.NewRouter()

	fs := http.FileServer(http.FS(staticFs))

	r.PathPrefix(siteRoot + "/static/").Handler(http.StripPrefix(siteRoot, fs))

	r.Path(siteRoot + "/").
		Methods(http.MethodGet).
		HandlerFunc(handler.Index)

	r.Path(siteRoot + "/export/").
		Methods(http.MethodGet).
		HandlerFunc(handler.Export)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":10000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Printf("Server listening on :10000")
	log.Fatal(srv.ListenAndServe())
}
