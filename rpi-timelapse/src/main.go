package main

import (
	"os"
	"time"
)

func main() {

	siteRoot := os.Getenv("RPI_CAMERA_SITE_ROOT")
	if siteRoot == "" {
		siteRoot = ""
	}

	storageDirectory := os.Getenv("RPI_CAMERA_STORAGE_DIR")
	if storageDirectory == "" {
		storageDirectory = "./data"
	}

	store := &TimelapseStore{
		StorageDirectory: storageDirectory,
	}

	capturer := &ImageCapturer{}

	timelapse := &TimelapseSettings{
		Name:     "test",
		Interval: (time.Duration(30) * time.Second),
		Camera: CameraSettings{
			HFlip:  false,
			VFlip:  false,
			Width:  640,
			Height: 480,
		},
	}

	timelapseCamera := &TimelapseCamera{
		ImageCapturer: capturer,
		Store:         store,
	}

	store.Init()
	store.SetCurrentTimelapse(timelapse)

	timelapseCamera.StartTimelapse(timelapse)

	handleRequests(
		siteRoot,
		store,
		capturer,
	)
}
