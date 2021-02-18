package main

import (
	"bufio"
	"os"
	"time"
)

type SpeedTestResultStore struct {
	File string
}

type SpeedTestSummary struct {
	LastDay   SummaryEntry
	LastWeek  SummaryEntry
	LastMonth SummaryEntry
}

type SummaryEntry struct {
	AveragePing          time.Duration
	AverageDownloadSpeed float64
	AverageUploadSpeed   float64
}

func FilterResultsWithinLast(results []*SpeedTestResult, dur time.Duration) []*SpeedTestResult {
	filtered := make([]*SpeedTestResult, 0)

	for _, result := range results {
		if result.Time.After(time.Now().Add(-dur)) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func GenerateSubSummary(results []*SpeedTestResult) SummaryEntry {
	pingCount := 0.0
	downloadCount := 0.0
	uploadCount := 0.0

	pingSum := time.Duration(0)
	downloadSum := 0.0
	uploadSum := 0.0

	for _, result := range results {
		if result.Latency != 0 {
			pingCount++
		}
		if result.DownloadSpeed != 0 {
			downloadCount++
		}
		if result.UploadSpeed != 0 {
			uploadCount++
		}
		pingSum += result.Latency
		downloadSum += result.DownloadSpeed
		uploadSum += result.UploadSpeed
	}

	return SummaryEntry{
		AveragePing:          time.Duration(float64(pingSum.Milliseconds())/pingCount) * time.Millisecond,
		AverageDownloadSpeed: downloadSum / downloadCount,
		AverageUploadSpeed:   uploadSum / uploadCount,
	}
}

func GenerateSummary(results []*SpeedTestResult) SpeedTestSummary {

	return SpeedTestSummary{
		LastDay:   GenerateSubSummary(FilterResultsWithinLast(results, time.Duration(24)*time.Hour)),
		LastWeek:  GenerateSubSummary(FilterResultsWithinLast(results, time.Duration(24*7)*time.Hour)),
		LastMonth: GenerateSubSummary(FilterResultsWithinLast(results, time.Duration(24*30)*time.Hour)),
	}

}

func (store *SpeedTestResultStore) Add(result *SpeedTestResult) {
	f, _ := os.OpenFile(store.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	f.Write([]byte(result.ToCsv() + "\n"))
	f.Close()
}

func (store *SpeedTestResultStore) GetAll() *SpeedTestResults {
	f, err := os.OpenFile(store.File, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		panic(store.File + " did not exist")
	}

	results := make([]*SpeedTestResult, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		results = append(results, SpeedTestResultFromCsv(scanner.Text()))
	}

	f.Close()

	return &SpeedTestResults{
		Entries: results,
		Summary: GenerateSummary(results),
	}
}
