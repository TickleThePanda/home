package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

func main() {

	tester := &SpeedTester{
		Store: &SpeedTestResultStore{
			File: GetEnvOrDefault("SPEED_TEST_STORE", "/data/store.csv"),
		},
		EmailConfig: &EmailConfig{
			SendGridApiKey: os.Getenv("SPEED_TEST_SENDGRID_API_KEY"),
			EmailThreshold: GetEnvAsFloat("SPEED_TEST_EMAIL_THRESHOLD", 0),
			EmailTo:        os.Getenv("SPEED_TEST_EMAIL_TO"),
			EmailFrom:      os.Getenv("SPEED_TEST_EMAIL_FROM"),
		},
		TestPeriod: int32(GetEnvAsInt("SPEED_TEST_PERIOD", 60*60)),
		AlertConfig: &AlertConfig{
			CronExpresion: GetEnvOrDefault("SPEED_TEST_EMAIL_CRON", "@monthly"),
			AlertType:     GetEnvOrDefault("SPEED_TEST_REPORT_PERIOD", "month"),
		},
	}

	log.Printf("Tester config %+v\n", tester)

	go tester.startTests()
	go tester.startEmailer()

	handleRequests(
		tester,
		os.Getenv("SPEED_TEST_SITE_ROOT"),
		os.Getenv("SPEED_TEST_SHARED_ASSETS_SITE"),
	)
}

func GetEnvAsInt(env string, defaultValue int64) int64 {

	valueAsText := os.Getenv(env)

	if valueAsText == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueAsText, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse %v to integer", env))
	}
	return value
}

func GetEnvAsFloat(env string, defaultValue float64) float64 {

	valueAsText := os.Getenv(env)

	if valueAsText == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(valueAsText, 64)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse %v to float", env))
	}
	return value
}

func GetEnvOrDefault(env string, defaultValue string) string {
	value := os.Getenv(env)
	if value != "" {
		return value
	} else {
		return defaultValue
	}
}
