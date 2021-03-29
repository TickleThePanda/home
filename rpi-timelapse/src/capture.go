package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dhowden/raspicam"
)

type ImageCapturer struct{}

func (c *ImageCapturer) CaptureImage(cameraSettings *CameraSettings, w io.Writer) {
	s := raspicam.NewStill()
	s.Camera.VFlip = cameraSettings.VFlip
	s.Camera.HFlip = cameraSettings.HFlip
	s.Width = cameraSettings.Width
	s.Height = cameraSettings.Height
	s.Encoding = raspicam.EncodingPNG

	errCh := make(chan error)
	go func() {
		for x := range errCh {
			fmt.Fprintf(os.Stderr, "%v\n", x)
		}
	}()
	raspicam.Capture(s, w, errCh)
}
