package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SpeedTestAggregate struct {
	Time          time.Time
	Distance      float64
	Latency       time.Duration
	DownloadSpeed float64
	UploadSpeed   float64
}

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

type ByDate []*SpeedTestResult

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Less(i, j int) bool { return a[i].Time.Before(a[j].Time) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
