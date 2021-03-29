package main

import (
	"log"
	"time"
)

type TimelapseCamera struct {
	Store         *TimelapseStore
	ImageCapturer *ImageCapturer
	CurrentTicker *time.Ticker
}

func (tt *TimelapseCamera) StartTimelapse(t *TimelapseSettings) {
	log.Printf("Starting timelapse ticker with interval %v", t.Interval)
	tt.CurrentTicker = time.NewTicker(t.Interval)
	go func() {
		for range tt.CurrentTicker.C {
			log.Printf("Taking image at triggered interval")
			err := tt.Store.StoreImage(tt.ImageCapturer.CaptureImage)
			if err != nil {
				log.Printf("Error storing image")
			}
		}
	}()

	tt.Store.StoreImage(tt.ImageCapturer.CaptureImage)
}
