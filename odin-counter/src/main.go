package main

import (
	"log"
	"os"
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

	var value int64
	_, err := parseIntFromString(valueAsText, &value)
	if err != nil {
		log.Printf("Warning: Unable to parse %v to integer, using default %d", env, defaultValue)
		return defaultValue
	}
	return value
}

func parseIntFromString(s string, result *int64) (int, error) {
	n, err := parseInt(s)
	if err != nil {
		return 0, err
	}
	*result = n
	return 1, nil
}

func parseInt(s string) (int64, error) {
	var result int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		result = result*10 + int64(c-'0')
	}
	return result, nil
}

func GetEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value != "" {
		return value
	}
	return defaultValue
}
