package main

import (
	"log"
	"os"
	"strconv"
)

func main() {

	storeFile := os.Getenv("SPEED_TEST_STORE")

	if storeFile == "" {
		storeFile = "/data/store.csv"
	}

	store := &SpeedTestResultStore{
		File: storeFile,
	}

	testPeriodText := os.Getenv("SPEED_TEST_PERIOD")

	var testPeriod int64
	testPeriod = 60 * 60
	if testPeriodText != "" {
		var err error
		testPeriod, err = strconv.ParseInt(testPeriodText, 10, 64)
		if err != nil {
			panic("Unable to parse SPEED_TEST_PERIOD to integer")
		}
	}

	siteRoot := os.Getenv("SPEED_TEST_SITE_ROOT")
	if siteRoot == "" {
		siteRoot = ""
	}

	sharedAssets := os.Getenv("SPEED_TEST_SHARED_ASSETS_SITE")
	if siteRoot == "" {
		sharedAssets = ""
	}

	log.Printf("Test period: %d", testPeriod)
	log.Printf("Site root: %s", siteRoot)

	tester := &SpeedTester{Store: store}

	go tester.startTests(int32(testPeriod))

	handleRequests(tester, siteRoot, sharedAssets)
}
