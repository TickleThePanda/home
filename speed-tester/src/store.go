package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type SpeedTestResults struct {
	Entries []*SpeedTestResult
	Summary SpeedTestSummary
}

func (rs *SpeedTestResults) RecentEntries() []*SpeedTestResult {
	recentEntries := FilterResultsWithinLast(rs.Entries, time.Duration(24)*time.Hour)
	sort.Sort(sort.Reverse(ByDate(recentEntries)))
	return recentEntries
}

func (rs *SpeedTestResults) LastMonth() []*SpeedTestAggregate {

	recentEntries := FilterResultsWithinLast(rs.Entries, time.Duration(24*30)*time.Hour)

	daysToEntries := make(map[string][]*SpeedTestResult)

	for _, recentEntry := range recentEntries {

		key := recentEntry.Time.Format("2006-01-02")

		if daysToEntries[key] == nil {
			daysToEntries[key] = make([]*SpeedTestResult, 0)
		}

		daysToEntries[key] = append(daysToEntries[key], recentEntry)

	}

	monthEntries := make([]*SpeedTestAggregate, 0)

	for key, entries := range daysToEntries {
		date, err := time.Parse("2006-01-02", key)
		if err != nil {
			panic(fmt.Sprintf("Error parsing date: %v", err))
		}
		distanceSum := float64(0)
		latencySum := time.Duration(0)
		downloadSpeedSum := float64(0)
		uploadSpeedSum := float64(0)

		for _, entry := range entries {
			distanceSum += entry.Distance
			latencySum += entry.Latency
			downloadSpeedSum += entry.DownloadSpeed
			uploadSpeedSum += entry.UploadSpeed
		}

		var count = float64(len(entries))

		monthEntries = append(monthEntries, &SpeedTestAggregate{
			Time:          date,
			Distance:      distanceSum / count,
			Latency:       time.Duration(float64(latencySum.Microseconds())/count) * time.Microsecond,
			DownloadSpeed: downloadSpeedSum / float64(len(entries)),
			UploadSpeed:   uploadSpeedSum / float64(len(entries)),
		})
	}

	return monthEntries
}

func (rs *SpeedTestResults) LastYear() []*SpeedTestAggregate {
	recentEntries := FilterResultsWithinLast(rs.Entries, time.Duration(24*365)*time.Hour)

	daysToEntries := make(map[string][]*SpeedTestResult)

	for _, recentEntry := range recentEntries {

		key := recentEntry.Time.Format("2006-01")

		if daysToEntries[key] == nil {
			daysToEntries[key] = make([]*SpeedTestResult, 0)
		}

		daysToEntries[key] = append(daysToEntries[key], recentEntry)

	}

	monthEntries := make([]*SpeedTestAggregate, 0)

	for key, entries := range daysToEntries {
		date, err := time.Parse("2006-01", key)
		if err != nil {
			panic(fmt.Sprintf("Error parsing date: %v", err))
		}
		distanceSum := float64(0)
		latencySum := time.Duration(0)
		downloadSpeedSum := float64(0)
		uploadSpeedSum := float64(0)

		for _, entry := range entries {
			distanceSum += entry.Distance
			latencySum += entry.Latency
			downloadSpeedSum += entry.DownloadSpeed
			uploadSpeedSum += entry.UploadSpeed
		}

		var count = float64(len(entries))

		monthEntries = append(monthEntries, &SpeedTestAggregate{
			Time:          date,
			Distance:      distanceSum / count,
			Latency:       time.Duration(float64(latencySum.Microseconds())/count) * time.Microsecond,
			DownloadSpeed: downloadSpeedSum / float64(len(entries)),
			UploadSpeed:   uploadSpeedSum / float64(len(entries)),
		})
	}

	return monthEntries
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

func (store *SpeedTestResultStore) Export(writer io.Writer) {
	f, err := os.OpenFile(store.File, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		panic(store.File + " did not exist")
	}

	io.Copy(writer, f)
}
