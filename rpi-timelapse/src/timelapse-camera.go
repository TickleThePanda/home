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
	s.Camera.MeteringMode = raspicam.MeteringMatrix
	s.Camera.AWBMode = raspicam.AWBCloudy
	s.Camera.ISO = 200
	s.Args = []string{"--flicker", "50hz"}
	s.Width = cameraSettings.Width
	s.Height = cameraSettings.Height
	s.Encoding = raspicam.EncodingPNG

	errCh := make(chan error)
	go func() {
		for x := range errCh {
			fmt.Fprintf(os.Stderr, "%v\n", x)
		}
	}()
	defer c.Mutex.Unlock()
	c.Mutex.Lock()
	raspicam.Capture(s, w, errCh)
}
