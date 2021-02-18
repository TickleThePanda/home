package main

import (
	"log"
	"os"
	"strconv"
)

func main() {

	store := &SpeedTestResultStore{
		File: "/data/store.csv",
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

	log.Printf("Test period: %d", testPeriod)

	go runTests(store, int32(testPeriod))

	handleRequests(store)
}
