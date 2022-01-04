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

func (e EmailConfig) String() string {
	return fmt.Sprintf(
		"[EmailThreshold: %v, SenderGridApiKey: ..., EmailTo: %v, EmailFrom %v]",
		e.EmailThreshold,
		e.EmailTo,
		e.EmailFrom,
	)
}

type AlertConfig struct {
	CronExpresion string
	AlertType     string
}

func (a AlertConfig) String() string {
	return fmt.Sprintf(
		"[CronExpression: %v, AlertType: %v]",
		a.CronExpresion,
		a.AlertType,
	)
}

type SpeedTester struct {
	Store       *SpeedTestResultStore
	EmailConfig *EmailConfig
	TestPeriod  int32
	AlertConfig *AlertConfig
}

func (s SpeedTester) String() string {
	return fmt.Sprintf(
		"[Store: %v, EmailConfig: %v, TestPeriod: %v, AlertConfig: %v]",
		s.Store,
		s.EmailConfig,
		s.TestPeriod,
		s.AlertConfig,
	)
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

func (tester *SpeedTester) startTests() {
	ticker := time.NewTicker(time.Duration(tester.TestPeriod) * time.Second)

	tester.runTestNow()
	for range ticker.C {
		tester.runTestNow()
	}
}

func (tester *SpeedTester) handleAlerts() {

	log.Println("Running alert handler")

	p, err := RecentPeriodFromString(tester.AlertConfig.AlertType)

	if err != nil {
		panic(err)
	}

	recentSpeed := tester.Store.GetResults().RecentSpeed(p)
	log.Println("Checking recent speed")
	if recentSpeed.DownloadSpeed90th < tester.EmailConfig.EmailThreshold {
		log.Println("Speed below threshold, sending email")

		subject := fmt.Sprintf(
			"Warning: speed below threshold (%v)",
			p.FormatDate(time.Now()),
		)

		content := fmt.Sprintf(
			"Actual speed %v was below %v.",
			recentSpeed.DownloadSpeed90th,
			tester.EmailConfig.EmailThreshold,
		)

		from := mail.NewEmail("Speed test alerts", tester.EmailConfig.EmailFrom)
		to := mail.NewEmail(tester.EmailConfig.EmailTo, tester.EmailConfig.EmailTo)
		plainTextContent := content
		htmlContent := content
		message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)
		client := sendgrid.NewSendClient(tester.EmailConfig.SendGridApiKey)
		response, err := client.Send(message)

		if err != nil {
			log.Printf("Failed to send email: %v\n", err)
		} else {
			if response.StatusCode != 202 {
				log.Printf("Failed to send email: %v, %v, %v\n", response.StatusCode, response.Body, response.Headers)
			} else {
				log.Println("Sent email")
			}
		}
	}
}

func (tester *SpeedTester) startEmailer() {

	log.Printf("Running emailer %v", tester.AlertConfig)

	c := cron.New()

	if !tester.EmailConfig.Complete() {
		log.Println("Not sending email, config not completed")
		return
	}

	_, e := c.AddFunc(tester.AlertConfig.CronExpresion, tester.handleAlerts)

	if e != nil {
		log.Printf("Error creating emailer %v", e)
	}
	c.Run()

}
