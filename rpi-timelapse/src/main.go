package main

import (
  "log"
  "os"
)

func main() {

  siteRoot := os.Getenv("RPI_CAMERA_SITE_ROOT")
  if siteRoot == "" {
    siteRoot = ""
  }
  sharedAssets := os.Getenv("RPI_CAMERA_SHARED_ASSETS_SITE")
  if siteRoot == "" {
    sharedAssets = ""
  }

  storageDirectory := os.Getenv("RPI_CAMERA_STORAGE_DIR")
  if storageDirectory == "" {
    storageDirectory = "./data"
  }

  store := &TimelapseStore{
    StorageDirectory: storageDirectory,
  }

  timelapseCamera := &TimelapseCamera{
    Store: store,
  }

  store.Init()

  timelapse, err := store.GetCurrentTimelapse()
  if err != nil {
    log.Printf("Unable to get current timelapse: %s", err.Error())
  } else {
    log.Printf("Starting existing timelapse %+v", timelapse)
    go timelapseCamera.StartTimelapse(timelapse)
  }

  handleRequests(
    &SiteInfo{
      SiteRoot:         siteRoot,
      SharedAssetsSite: sharedAssets,
    },
    store,
    timelapseCamera,
  )
}
