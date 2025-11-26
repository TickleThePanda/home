package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type FetchRecord struct {
	Time time.Time
}

type DailyCount struct {
	Date  string
	Count int
}

type OdinBotStore struct {
	File string
}

func (r *FetchRecord) ToCsv() string {
	return r.Time.Format(time.RFC3339)
}

func FetchRecordFromCsv(line string) *FetchRecord {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(line))
	if err != nil {
		return nil
	}
	return &FetchRecord{Time: t}
}

func (store *OdinBotStore) Add(record *FetchRecord) error {
	f, err := os.OpenFile(store.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write([]byte(record.ToCsv() + "\n"))
	return err
}

func (store *OdinBotStore) GetAllRecords() ([]*FetchRecord, error) {
	f, err := os.OpenFile(store.File, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records := make([]*FetchRecord, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		record := FetchRecordFromCsv(scanner.Text())
		if record != nil {
			records = append(records, record)
		}
	}

	return records, nil
}

func (store *OdinBotStore) GetDailyCounts() ([]DailyCount, error) {
	records, err := store.GetAllRecords()
	if err != nil {
		return nil, err
	}

	countsByDate := make(map[string]int)
	for _, record := range records {
		dateKey := record.Time.Format("2006-01-02")
		countsByDate[dateKey]++
	}

	counts := make([]DailyCount, 0, len(countsByDate))
	for date, count := range countsByDate {
		counts = append(counts, DailyCount{Date: date, Count: count})
	}

	sort.Slice(counts, func(i, j int) bool {
		return counts[i].Date > counts[j].Date
	})

	return counts, nil
}

func (store *OdinBotStore) GetTodayCount() (int, error) {
	f, err := os.OpenFile(store.File, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	today := time.Now().Format("2006-01-02")
	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		record := FetchRecordFromCsv(scanner.Text())
		if record != nil && record.Time.Format("2006-01-02") == today {
			count++
		}
	}

	return count, nil
}

func (store *OdinBotStore) Export(writer io.Writer) error {
	counts, err := store.GetDailyCounts()
	if err != nil {
		return err
	}

	// Write header
	if _, err := writer.Write([]byte("date,count\n")); err != nil {
		return err
	}

	for _, count := range counts {
		line := fmt.Sprintf("%s,%s\n", count.Date, strconv.Itoa(count.Count))
		if _, err := writer.Write([]byte(line)); err != nil {
			return err
		}
	}

	return nil
}
