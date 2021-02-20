package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type SpeedTestResult struct {
	Time          time.Time
	ServerId      string
	ServerName    string
	Distance      float64
	Latency       time.Duration
	DownloadSpeed float64
	UploadSpeed   float64
}

func (r *SpeedTestResult) String() string {
	return fmt.Sprintf(
		"%s, %s, %s, %f, %s, %f, %f",
		r.Time,
		r.ServerId, r.ServerName,
		r.Distance, r.Latency,
		r.DownloadSpeed, r.UploadSpeed,
	)
}

func (r *SpeedTestResult) ToCsv() string {
	return fmt.Sprintf(
		"%s,%s,%s,%f,%s,%f,%f",
		r.Time.Format(time.RFC3339),
		r.ServerId, r.ServerName,
		r.Distance, r.Latency.String(),
		r.DownloadSpeed, r.UploadSpeed,
	)
}

func SpeedTestResultFromCsv(csv string) *SpeedTestResult {
	sp := strings.Split(csv, ",")

	t, _ := time.Parse(time.RFC3339, sp[0])
	dist, _ := strconv.ParseFloat(sp[3], 10)
	latency, _ := time.ParseDuration(sp[4])
	down, _ := strconv.ParseFloat(sp[5], 10)
	up, _ := strconv.ParseFloat(sp[6], 10)

	return &SpeedTestResult{
		Time:          t,
		ServerId:      sp[1],
		ServerName:    sp[2],
		Distance:      dist,
		Latency:       latency,
		DownloadSpeed: down,
		UploadSpeed:   up,
	}
}

type SpeedTestResults struct {
	Entries []*SpeedTestResult
	Summary SpeedTestSummary
}

func (rs *SpeedTestResults) RecentEntries() []*SpeedTestResult {
	return FilterResultsWithinLast(rs.Entries, time.Duration(24)*time.Hour)
}

func (rs *SpeedTestResults) String() string {
	var b strings.Builder
	for _, r := range rs.Entries {
		b.WriteString(r.String())
		b.WriteString("\n")
	}
	return b.String()
}

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

func (store *SpeedTestResultStore) GetResults() *SpeedTestResults {
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
