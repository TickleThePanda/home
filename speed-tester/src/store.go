package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

type SpeedTestResults struct {
	Entries []*SpeedTestResult
	Summary SpeedTestSummary
}

//go:generate stringer -type=RecentPeriod
type RecentPeriod int

const (
	Month RecentPeriod = iota
	Week
	Day
)

func (p *RecentPeriod) GetDuration() time.Duration {
	switch *p {
	case Month:
		return time.Duration(24*30) * time.Hour
	case Week:
		return time.Duration(24*7) * time.Hour
	case Day:
		return time.Duration(24) * time.Hour
	default:
		panic("recent period not recognised")
	}
}

func (p *RecentPeriod) FormatDate(t time.Time) string {
	switch *p {
	case Month:
		return t.Format("Jan 2006")
	case Week:
		return t.Format("2 Jan 2006")
	case Day:
		return t.Format("2 Jan 2006")
	default:
		panic("recent period not recognised")
	}
}

func RecentPeriodFromString(p string) (RecentPeriod, error) {
	switch p {
	case "":
		return Day, nil
	case "month":
		return Month, nil
	case "week":
		return Week, nil
	case "day":
		return Day, nil
	default:
		return -1, fmt.Errorf("no recent period called %s", p)
	}
}

func (rs *SpeedTestResults) RecentEntries(period RecentPeriod) []*SpeedTestResult {

	recentEntries := FilterResultsWithinLast(rs.Entries, period.GetDuration())
	sort.Sort(sort.Reverse(ByDate(recentEntries)))
	return recentEntries
}

func (rs *SpeedTestResults) RecentSpeed(period RecentPeriod) *SpeedTestAggregate {
	return AggregateForEntries(time.Now(), rs.RecentEntries(period))
}

func AggregateForEntries(date time.Time, entries []*SpeedTestResult) *SpeedTestAggregate {

	sort.Sort(ByDownloadSpeed(entries))

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

	downloadSpeedMedian := entries[int(float64(len(entries))*0.5)].DownloadSpeed
	downloadSpeed90th := entries[int(float64(len(entries))*0.9)].DownloadSpeed

	var count = float64(len(entries))

	return &SpeedTestAggregate{
		Time:                date,
		DistanceMean:        distanceSum / count,
		LatencyMean:         time.Duration(float64(latencySum.Microseconds())/count) * time.Microsecond,
		DownloadSpeedMean:   downloadSpeedSum / float64(len(entries)),
		DownloadSpeedMedian: downloadSpeedMedian,
		DownloadSpeed90th:   downloadSpeed90th,
		UploadSpeedMean:     uploadSpeedSum / float64(len(entries)),
	}
}

func (rs *SpeedTestResults) AggregateBy(format string, since time.Duration) []*SpeedTestAggregate {

	recentEntries := FilterResultsWithinLast(rs.Entries, since)

	groupToEntries := make(map[string][]*SpeedTestResult)

	for _, recentEntry := range recentEntries {

		key := recentEntry.Time.Format(format)

		if groupToEntries[key] == nil {
			groupToEntries[key] = make([]*SpeedTestResult, 0)
		}

		groupToEntries[key] = append(groupToEntries[key], recentEntry)

	}

	aggregates := make([]*SpeedTestAggregate, 0)

	for key, entries := range groupToEntries {

		date, err := time.Parse(format, key)
		if err != nil {
			panic(fmt.Sprintf("Error parsing date: %v", err))
		}
		aggregates = append(aggregates, AggregateForEntries(date, entries))
	}

	return aggregates

}

func (rs *SpeedTestResults) LastMonth() []*SpeedTestAggregate {
	return rs.AggregateBy("2006-01-02", time.Duration(24*30)*time.Hour)
}

func (rs *SpeedTestResults) LastYear() []*SpeedTestAggregate {
	return rs.AggregateBy("2006-01", time.Duration(24*365)*time.Hour)
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
