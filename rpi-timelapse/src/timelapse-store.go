package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"

	"github.com/gosimple/slug"
)

type TimelapseStore struct {
	StorageDirectory string
	CurrentTimelapse *TimelapseSettings
}

type CameraSettings struct {
	HFlip  bool
	VFlip  bool
	Width  int
	Height int
}

type TimelapseSettings struct {
	Name     string
	Interval time.Duration
	Camera   CameraSettings
}

func (store *TimelapseStore) TimelapseDir(t *TimelapseSettings) string {
	return store.StorageDirectory + "/" + slug.Make(t.Name)
}
func (store *TimelapseStore) TimelapseImageDir(t *TimelapseSettings) string {
	return store.TimelapseDir(t) + "/images"
}

func (store *TimelapseStore) Init() error {
	err := os.MkdirAll(store.StorageDirectory, 0755)
	if err != nil {
		return fmt.Errorf("unable to create data directory: %w", err)
	}
	return nil
}

func (store *TimelapseStore) SetCurrentTimelapse(t *TimelapseSettings) (*TimelapseSettings, error) {
	file, err := os.OpenFile(store.StorageDirectory+"/current", os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, fmt.Errorf("unable to start new timelapse: %w", err)
	}

	enc := gob.NewEncoder(file)
	encErr := enc.Encode(t)

	if encErr != nil {
		return nil, fmt.Errorf("unable to start new timelapse: %w", err)
	}

	os.Mkdir(store.TimelapseDir(t), 0755)

	store.CurrentTimelapse = t

	return t, nil
}

func (store *TimelapseStore) GetCurrentTimelapse() (*TimelapseSettings, error) {
	if store.CurrentTimelapse == nil {
		file, err := os.OpenFile(store.StorageDirectory+"/current", os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return nil, fmt.Errorf("unable to get current timelapse: %w", err)
		}

		dec := gob.NewDecoder(file)
		var t TimelapseSettings
		decErr := dec.Decode(&t)

		log.Printf("Current timelapse: %+v", t)

		if decErr != nil {
			return nil, fmt.Errorf("unable to get current timelapse: %w", err)
		}

		return &t, nil
	} else {
		return store.CurrentTimelapse, nil
	}
}

func (store *TimelapseStore) StoreImage(imageCapturer func(*CameraSettings, io.Writer)) error {

	t, err := store.GetCurrentTimelapse()

	if err != nil {
		return fmt.Errorf("unable to get latest timelapse: %w", err)
	}

	fileName := time.Now().Format(time.RFC3339)

	filePath := store.TimelapseImageDir(t) + "/" + fileName

	log.Printf("Saving file: %s", filePath)
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)

	if err != nil {
		return fmt.Errorf("unable to store new image: %w", err)
	}

	imageCapturer(&t.Camera, file)

	return nil
}

func (store *TimelapseStore) ImageNames() ([]string, error) {

	t, err := store.GetCurrentTimelapse()

	if err != nil {
		return nil, fmt.Errorf("unable to get image names: %w", err)
	}

	path := store.TimelapseImageDir(t)

	dir, err := os.OpenFile(path, os.O_RDONLY, 0755)
	if err != nil {
		return nil, fmt.Errorf("unable to get image names: %w", err)
	}

	files, err := dir.Readdirnames(0)
	if err != nil {
		return nil, fmt.Errorf("unable to get image names: %w", err)
	}

	sort.Strings(files)

	return files, nil

}

func (store *TimelapseStore) LatestImage(w io.Writer) error {

	files, err := store.ImageNames()
	if err != nil {
		return fmt.Errorf("unable to get latest timelapse: %w", err)
	}

	lastFile := files[len(files)-1]

	return store.ImageByName(lastFile, w)

}

func (store *TimelapseStore) ImageByName(name string, w io.Writer) error {

	t, err := store.GetCurrentTimelapse()
	if err != nil {
		return fmt.Errorf("unable to get latest timelapse: %w", err)
	}

	path := store.TimelapseImageDir(t)

	imageFile, err := os.OpenFile(path+"/"+name, os.O_RDONLY, 0755)
	log.Printf("Last file: %v", imageFile.Name())

	if err != nil {
		return fmt.Errorf("unable to get latest timelapse: %w", err)
	}

	io.Copy(w, imageFile)

	return nil
}
