package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/dhowden/raspicam"
)

type TimelapseCamera struct {
	Store         *TimelapseStore
	CurrentTicker *time.Ticker
	Mutex         sync.Mutex
}

func (tt *TimelapseCamera) StartTimelapse(t *TimelapseSettings) {

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

func (c *TimelapseCamera) CaptureImage(cameraSettings *CameraSettings, w io.Writer) {
	log.Printf("Camera settings for capture: %+v", cameraSettings)
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
	s.Timeout = time.Duration(1) * time.Minute

	errCh := make(chan error)
	go func() {
		for x := range errCh {
			fmt.Fprintf(os.Stderr, "%v\n", x)
		}
	}()
	c.Mutex.Lock()
	log.Println("Reserved camera")
	defer func() {
		c.Mutex.Unlock()
		log.Println("Freed camera")
	}()
	raspicam.Capture(s, w, errCh)
}
