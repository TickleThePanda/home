package main

import (
	"log"
	"net/http"
	"time"
)

type OdinFetcher struct {
	Store         *OdinBotStore
	TargetURL     string
	FetchInterval int
}

func (f *OdinFetcher) Start() {
	log.Printf("Starting Odin fetcher, fetching %s every %d seconds", f.TargetURL, f.FetchInterval)

	// Do an initial fetch immediately
	f.fetch()

	ticker := time.NewTicker(time.Duration(f.FetchInterval) * time.Second)
	for range ticker.C {
		f.fetch()
	}
}

func (f *OdinFetcher) fetch() {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, f.TargetURL, nil)
	if err != nil {
		log.Printf("Error creating request for %s: %v", f.TargetURL, err)
		return
	}
	req.Header.Set("User-Agent", "OdinBot https://home.ticklethepanda.co.uk/odinbot/")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error fetching %s: %v", f.TargetURL, err)
		return
	}
	defer resp.Body.Close()

	record := &FetchRecord{
		Time: time.Now(),
	}

	if err := f.Store.Add(record); err != nil {
		log.Printf("Error storing fetch record: %v", err)
		return
	}

	log.Printf("Successfully fetched %s (status: %d)", f.TargetURL, resp.StatusCode)
}
