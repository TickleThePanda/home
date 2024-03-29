package main

import (
  "bytes"
  "encoding/gob"
  "fmt"
  "log"
  "os"
  "sort"
  "time"
  "io/ioutil"

  "github.com/gosimple/slug"
)

type TimelapseStore struct {
  StorageDirectory string
  CurrentTimelapse *TimelapseSettings
  OpenFiles        map[string]bool
}

type CameraSettings struct {
  HFlip    bool
  VFlip    bool
  Width    int
  Height   int
  Rotation int
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

  store.OpenFiles = make(map[string]bool)

  err := os.MkdirAll(store.StorageDirectory, 0755)
  if err != nil {
    return fmt.Errorf("unable to create data directory: %w", err)
  }
  return nil
}

func (store *TimelapseStore) InitTimelapseDirs() error {
  t, err := store.GetCurrentTimelapse()
  if err != nil {
    return fmt.Errorf("unable to init timelapse dirs: %w", err)
  }

  timelapseDir := os.MkdirAll(store.TimelapseDir(t), 0755)
  if timelapseDir != nil {
    return fmt.Errorf("unable to init timelapse dirs: %w", timelapseDir)
  }
  timelapseImageDir := os.MkdirAll(store.TimelapseImageDir(t), 0755)
  if timelapseImageDir != nil {
    return fmt.Errorf("unable to init timelapsedirs: %w", timelapseImageDir)
  }

  return nil
}

func (store *TimelapseStore) SetCurrentTimelapse(t *TimelapseSettings) (*TimelapseSettings, error) {
  file, err := os.OpenFile(store.StorageDirectory+"/current", os.O_RDWR|os.O_CREATE, 0755)
  defer func() {
    err := file.Close()
    if err != nil {
      log.Printf("Error closing timelapse file: %s", err.Error())
    }
  }()
  if err != nil {
    return nil, fmt.Errorf("unable to start new timelapse: %w", err)
  }

  enc := gob.NewEncoder(file)
  encErr := enc.Encode(t)

  if encErr != nil {
    return nil, fmt.Errorf("unable to start new timelapse: %w", err)
  }

  err = store.InitTimelapseDirs()
  if err != nil {
    return nil, fmt.Errorf("unable to start new timelapse: %w", err)
  }

  store.CurrentTimelapse = t

  return t, nil
}

func (store *TimelapseStore) GetCurrentTimelapse() (*TimelapseSettings, error) {
  if store.CurrentTimelapse == nil {
    file, err := os.OpenFile(store.StorageDirectory+"/current", os.O_RDWR|os.O_CREATE, 0755)
    defer func() {
      err := file.Close()
      if err != nil {
        log.Printf("Error closing timelapse file: %s", err.Error())
      }
    }()
    if err != nil {
      return nil, fmt.Errorf("unable to get current timelapse: %w", err)
    }

    dec := gob.NewDecoder(file)
    var t TimelapseSettings
    decErr := dec.Decode(&t)

    if decErr != nil {
      return nil, fmt.Errorf("unable to get current timelapse: %w", err)
    }

    return &t, nil
  } else {
    return store.CurrentTimelapse, nil
  }
}

func (store *TimelapseStore) StoreImage(imageCapturer func(*CameraSettings) ([]byte, error)) error {

  err := store.InitTimelapseDirs()
  if err != nil {
    return fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  t, err := store.GetCurrentTimelapse()
  if err != nil {
    return fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  filePath := store.TimelapseImageDir(t) + "/" + time.Now().Format(time.RFC3339)

  file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
  store.OpenFiles[filePath] = true
  defer func() {
    delete(store.OpenFiles, filePath)
    err := file.Close()
    if err != nil {
      log.Printf("Unable to close file: %s", err.Error())
    }
  }()

  if err != nil {
    return fmt.Errorf("unable to store new image: %w", err)
  }

  image, captureErr := imageCapturer(&t.Camera)
  _, writeErr := file.Write(image)

  if captureErr != nil || writeErr != nil {
    log.Printf("Deleting failed image %v", filePath)
    removeErr := os.Remove(filePath)
    if removeErr != nil {
      return fmt.Errorf("unable to capture image and clean up after: %w and %v", captureErr, removeErr)
    }
    return fmt.Errorf("unable to capture image: %w", captureErr)
  }

  return nil
}

func (store *TimelapseStore) ImageNames() ([]string, error) {

  t, err := store.GetCurrentTimelapse()

  if err != nil {
    return nil, fmt.Errorf("unable to get image names: %w", err)
  }

  path := store.TimelapseImageDir(t)

  dir, err := os.OpenFile(path, os.O_RDONLY, 0755)
  defer func() {
    err := dir.Close()
    if err != nil {
      log.Printf("Unable to close timelapse dir: %s", err.Error())
    }
  }()

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

func (store *TimelapseStore) LatestImage() ([]byte, error) {

  files, err := store.ImageNames()
  if err != nil {
    return nil, fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  lastFile := files[len(files)-1]

  return store.ImageByName(lastFile)

}

func (store *TimelapseStore) ImageByName(name string) ([]byte, error) {

  t, err := store.GetCurrentTimelapse()
  if err != nil {
    return nil, fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  path := store.TimelapseImageDir(t) + "/" + name

  if store.OpenFiles[path] {
    log.Printf("Waiting for image to be written: %s", path)
    for store.OpenFiles[path] {
      time.Sleep(time.Duration(100) * time.Millisecond)
    }
    log.Printf("Finished waiting for image to be written: %s", path)
  }

  imageFile, err := os.OpenFile(path, os.O_RDONLY, 0755)
  defer func() {
    err := imageFile.Close()
    if err != nil {
      log.Printf("unable to close file: %s", err.Error())
    }
  }()

  if err != nil {
    return nil, fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  buf := new(bytes.Buffer)
  _, err = buf.ReadFrom(imageFile)

  if err != nil {
    return nil, fmt.Errorf("unable to get latest timelapse: %w", err)
  }

  return buf.Bytes(), nil
}

func (store *TimelapseStore) GetTimelapses() ([]string, error) {
  files, err := ioutil.ReadDir(store.StorageDirectory)
  if (err != nil) {
    return nil, fmt.Errorf("unable to list timelapses: %w", err)
  }

  timelapses := []string{}

  for _, file := range files {
    if file.IsDir() {
      timelapses = append(timelapses, file.Name())
    }
  }

  return timelapses, nil

}
