package main

import (
	"log"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

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
