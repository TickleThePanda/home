package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/raspicam"
)

type TimelapseCamera struct {
	Store            *TimelapseStore
	CurrentTicker    *time.Ticker
	Mutex            sync.Mutex
	RequestsChannel  chan<- *CameraSettings
	ResponsesChannel <-chan ImageResult
}

func (tt *TimelapseCamera) StartTimelapse(t *TimelapseSettings) {

	reqs, res := tt.CameraServer()
	tt.RequestsChannel = reqs
	tt.ResponsesChannel = res

	if tt.CurrentTicker != nil {
		tt.CurrentTicker.Stop()
		tt.CurrentTicker = nil
	}

	log.Printf("Starting timelapse ticker with interval %v", t.Interval)
	tt.CurrentTicker = time.NewTicker(t.Interval)
	go func() {
		for range tt.CurrentTicker.C {
			log.Printf("Taking image at triggered interval")
			err := tt.Store.StoreImage(tt.CaptureImage)
			if err != nil {
				log.Printf("Error storing image: %s", err.Error())
			}
		}
	}()

	err := tt.Store.StoreImage(tt.CaptureImage)
	if err != nil {
		log.Printf("Error storing image: %s", err.Error())
	}

}

type ImageResult struct {
	Reader *bytes.Reader
	Error  error
}

func (c *TimelapseCamera) CameraServer() (requests chan<- *CameraSettings, responses <-chan ImageResult) {
	reqs := make(chan *CameraSettings, 1)
	ress := make(chan ImageResult, 1)

	go func() {
		for cameraSettings := range reqs {

			s := raspicam.NewStill()
			s.Camera.VFlip = cameraSettings.VFlip
			s.Camera.HFlip = cameraSettings.HFlip
			s.Camera.Rotation = cameraSettings.Rotation
			s.Camera.MeteringMode = raspicam.MeteringAverage
			s.Camera.AWBMode = raspicam.AWBOff
			s.Camera.ISO = 200
			s.Args = []string{"--flicker", "50hz", "-awbg", "1.7,1.9", "--drc", "high"}
			s.Width = cameraSettings.Width
			s.Height = cameraSettings.Height
			s.Encoding = raspicam.EncodingPNG

			errCh := make(chan error)

			wasError := false
			var imageResultError strings.Builder
			go func() {
				for x := range errCh {
					wasError = true
					imageResultError.WriteString(x.Error())
					imageResultError.WriteRune('\n')
				}
			}()

			var b *bytes.Buffer = &bytes.Buffer{}
			log.Println("Capturing image")
			raspicam.Capture(s, b, errCh)
			log.Println("Returning image")

			if wasError {
				ress <- ImageResult{
					Error: errors.New(imageResultError.String()),
				}
			} else {
				ress <- ImageResult{
					Reader: bytes.NewReader(b.Bytes()),
				}
			}
		}
	}()

	return reqs, ress
}

func (c *TimelapseCamera) CaptureImage(cameraSettings *CameraSettings, w io.Writer) error {

	log.Println("Requesting image")
	c.RequestsChannel <- cameraSettings
	result := <-c.ResponsesChannel

	if result.Error != nil {
		return fmt.Errorf("error getting image: %w", result.Error)
	}

	log.Println("Receiving image")
	_, err := io.Copy(w, result.Reader)
	if err != nil {
		return fmt.Errorf("error copying: %w", err)
	}

	return nil
}
