package main

import (
	"log"
	"os"
	"strconv"
)

func main() {
	storeFile := GetEnvOrDefault("ODINBOT_STORE", "/data/store.csv")
	floofStoreFile := GetEnvOrDefault("ODINBOT_FLOOF_STORE", "/data/floof_scores.csv")
	siteRoot := os.Getenv("ODINBOT_SITE_ROOT")
	sharedAssetsSite := os.Getenv("ODINBOT_SHARED_ASSETS_SITE")
	targetURL := GetEnvOrDefault("ODINBOT_TARGET_URL", "https://matt-vps.com/odin_of_the_day/")
	fetchInterval := GetEnvAsInt("ODINBOT_FETCH_INTERVAL", 10)

	store := &OdinBotStore{
		File: storeFile,
	}

	floofStore, err := NewFloofMajestyStore(floofStoreFile)
	if err != nil {
		log.Fatalf("Failed to initialise Floof Majesty store: %v", err)
	}

	floofEvaluator := NewFloofMajestyEvaluator(floofStore)

	fetcher := &OdinFetcher{
		Store:          store,
		TargetURL:      targetURL,
		FetchInterval:  int(fetchInterval),
		FloofEvaluator: floofEvaluator,
	}

	log.Printf("Odin Bot starting...")
	log.Printf("Store file: %s", storeFile)
	log.Printf("Floof Majesty store file: %s", floofStoreFile)
	log.Printf("Target URL: %s", targetURL)
	log.Printf("Fetch interval: %d seconds", fetchInterval)
	log.Printf("Site root: %s", siteRoot)

	go fetcher.Start()

	handleRequests(store, floofStore, siteRoot, sharedAssetsSite)
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
