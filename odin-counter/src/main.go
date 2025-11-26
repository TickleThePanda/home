package main

import (
	"log"
	"os"
	"strconv"
)

func main() {
	storeFile := GetEnvOrDefault("ODIN_COUNTER_STORE", "/data/store.csv")
	siteRoot := os.Getenv("ODIN_COUNTER_SITE_ROOT")
	targetURL := GetEnvOrDefault("ODIN_COUNTER_TARGET_URL", "https://matt-vps.com/odin_of_the_day/")
	fetchInterval := GetEnvAsInt("ODIN_COUNTER_FETCH_INTERVAL", 10)

	store := &OdinCounterStore{
		File: storeFile,
	}

	fetcher := &OdinFetcher{
		Store:         store,
		TargetURL:     targetURL,
		FetchInterval: int(fetchInterval),
	}

	log.Printf("Odin Counter starting...")
	log.Printf("Store file: %s", storeFile)
	log.Printf("Target URL: %s", targetURL)
	log.Printf("Fetch interval: %d seconds", fetchInterval)
	log.Printf("Site root: %s", siteRoot)

	go fetcher.Start()

	handleRequests(store, siteRoot)
}

func GetEnvAsInt(env string, defaultValue int64) int64 {
	valueAsText := os.Getenv(env)

	if valueAsText == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueAsText, 10, 64)
	if err != nil {
		log.Printf("Warning: Unable to parse %v to integer, using default %d", env, defaultValue)
		return defaultValue
	}
	return value
}

func GetEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value != "" {
		return value
	}
	return defaultValue
}
