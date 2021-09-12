package main

import (
	"fmt"
	"log"
	"time"

	"github.com/showwin/speedtest-go/speedtest"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"

	"github.com/robfig/cron/v3"
)

type EmailConfig struct {
	EmailThreshold float64
	SendGridApiKey string
	EmailTo        string
	EmailFrom      string
}

type SpeedTester struct {
	Store       *SpeedTestResultStore
	EmailConfig *EmailConfig
}

func (ec *EmailConfig) Complete() bool {
	return ec.EmailThreshold != 0.0 &&
		ec.SendGridApiKey != "" &&
		ec.EmailFrom != "" &&
		ec.EmailTo != ""
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

	return speed
}

func (tester *SpeedTester) runTestNow() *SpeedTestResult {

	result := testSpeed()
	tester.Store.Add(result)

	return result
}

func (tester *SpeedTester) startTests(periodInSeconds int32) {
	ticker := time.NewTicker(time.Duration(periodInSeconds) * time.Second)

	tester.runTestNow()
	for range ticker.C {
		tester.runTestNow()
	}
}

func (tester *SpeedTester) handleAlerts() {

	if !tester.EmailConfig.Complete() {
		println("Not sending email, config not completed")
		return
	}

	recentSpeed := tester.Store.GetResults().RecentSpeed()
	println("Checking recent speed")
	if recentSpeed.DownloadSpeed90th < tester.EmailConfig.EmailThreshold {
		println("Speed below threshold, sending email")

		content := fmt.Sprintf("Speed below threshold. %v.", recentSpeed.DownloadSpeed90th)

		from := mail.NewEmail("Speed test alerts", tester.EmailConfig.EmailFrom)
		subject := content
		to := mail.NewEmail(tester.EmailConfig.EmailTo, tester.EmailConfig.EmailTo)
		plainTextContent := content
		htmlContent := content
		message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
		client := sendgrid.NewSendClient(tester.EmailConfig.SendGridApiKey)
		response, err := client.Send(message)

		if err != nil {
			log.Printf("Failed to send email: %v", err)
		} else {
			if response.StatusCode != 202 {
				println("Failed to send email: %v, %v, %v", response.StatusCode, response.Body, response.Headers)
			} else {
				fmt.Printf("Sent email")
			}
		}
	}
}

func (tester *SpeedTester) startEmailer(cronExpression string) {
	c := cron.New()

	c.AddFunc(cronExpression, tester.handleAlerts)
	c.Start()

}
