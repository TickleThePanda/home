package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
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

func (rs *SpeedTestResults) String() string {
	var b strings.Builder
	for _, r := range rs.Entries {
		b.WriteString(r.String())
		b.WriteString("\n")
	}
	return b.String()
}

func testSpeed() *SpeedTestResult {

	log.Print("Testing speed - fetching user info")
	user, _ := speedtest.FetchUserInfo()

	log.Print("Testing speed - fetching server list")
	serverList, _ := speedtest.FetchServerList(user)
	log.Print("Testing speed - choosing server")
	targets, _ := serverList.FindServer([]int{})

	log.Print("Testing speed - running test")
	var speed *SpeedTestResult

	if len(targets) == 1 {
		s := targets[0]

		log.Print("Testing speed - running ping test")
		err := s.PingTest()
		if err != nil {
			log.Print(err)
		}

		log.Print("Testing speed - running download test")
		err = s.DownloadTest(false)
		if err != nil {
			log.Print(err)
		}

		log.Print("Testing speed - running upload test")
		err = s.UploadTest(false)
		if err != nil {
			log.Print(err)
		}

		speed = &SpeedTestResult{
			Time:          time.Now(),
			ServerId:      s.ID,
			ServerName:    s.Name,
			Distance:      s.Distance,
			Latency:       s.Latency,
			DownloadSpeed: s.DLSpeed,
			UploadSpeed:   s.ULSpeed,
		}

	}

	log.Println(speed.String())

	return speed
}

func runTests(store *SpeedTestResultStore, periodInSeconds int32) {
	ticker := time.NewTicker(time.Duration(periodInSeconds) * time.Second)

	result := testSpeed()
	store.Add(result)

	for range ticker.C {
		result := testSpeed()
		store.Add(result)
	}
}
