package main

import (
    "fmt"
    "log"
    "time"
    "strings"
    "net/http"
    "html/template"
    "github.com/showwin/speedtest-go/speedtest"
)

type SpeedTestResultHandler struct {
  Data      *SpeedTestResults
  Template  *template.Template
}

func (sh *SpeedTestResultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
  sh.Template.Execute(w, sh.Data)
}

func handleRequests(results *SpeedTestResults) {
    t := template.New("index")
    t, _ = template.ParseFiles("src/templates/index.html")

    handler := &SpeedTestResultHandler{
        Data: results,
        Template: t,
    }

    http.Handle("/", handler)
    log.Fatal(http.ListenAndServe(":10000", nil))
}

type SpeedTestResult struct {
    Id             string
    Name           string
    Distance       float64
    Latency        time.Duration
    DownloadSpeed  float64
    UploadSpeed    float64
}

func (r *SpeedTestResult) String() string {
    return fmt.Sprintf(
        "%s, %s, %f, %s, %f, %f",
        r.Id, r.Name,
        r.Distance, r.Latency,
        r.DownloadSpeed, r.UploadSpeed,
    )
}

type SpeedTestResults struct {
    Entries []*SpeedTestResult
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

    if (len(targets) == 1) {
        s := targets[0]

				s.PingTest()
				s.DownloadTest(false)
				s.UploadTest(false)

        speed = &SpeedTestResult{
            Id: s.ID,
            Name: s.Name,
            Distance: s.Distance,
            Latency: s.Latency,
            DownloadSpeed: s.DLSpeed,
            UploadSpeed: s.ULSpeed,
        }

    }

    log.Println(speed.String())

    return speed
}

func runTests(results *SpeedTestResults) {
  ticker := time.NewTicker(180 * time.Second)

  result := testSpeed()
  results.Entries = append(results.Entries, result)

  for _ = range ticker.C {
    result := testSpeed()
    results.Entries = append(results.Entries, result)
  }
}

func main() {

    results := &SpeedTestResults{Entries: make([]*SpeedTestResult, 0)}

    go runTests(results)

    handleRequests(results)
}
