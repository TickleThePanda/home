package main

import (
  "embed"
  "fmt"
  "html/template"
  "log"
  "net/http"
  "strconv"
  "time"

  "github.com/gorilla/mux"
)

//go:embed templates/*
var templatesFs embed.FS

type ImageResultHander struct {
  SiteInfo  *SiteInfo
  Store     *TimelapseStore
  Camera    *TimelapseCamera
  Templates *template.Template
}

type Templates struct {
  Index  *template.Template
  Images *template.Template
}

type SiteInfo struct {
  SiteRoot         string
  SharedAssetsSite string
}

type ImageResponseData struct {
  Timestamp string
}

type ImagesPageResponseData struct {
  Images   []ImageResponseData
  SiteInfo *SiteInfo
}

type IndexPageResponseData struct {
  SiteInfo *SiteInfo
  Timelapses []string
}

var DEFAULT_CAMERA_SETTINGS CameraSettings = CameraSettings{
  HFlip:    false,
  VFlip:    false,
  Width:    1600,
  Height:   1080,
  Rotation: 180,
}

var NOW_CAMERA_SETTINGS CameraSettings = CameraSettings{
  HFlip:    false,
  VFlip:    false,
  Width:    640,
  Height:   480,
  Rotation: 180,
}

func (ih *ImageResultHander) GetLatestImage(w http.ResponseWriter, r *http.Request) {

  bytes, err := ih.Store.LatestImage()
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    w.Write([]byte("Internal server error"))
  } else {
    w.Header().Set("Content-Type", "image/png")
    w.WriteHeader(http.StatusOK)
    w.Write(bytes)
  }

}

func (ih *ImageResultHander) GetImageNamePage(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/html")
  w.WriteHeader(http.StatusOK)
  names, _ := ih.Store.ImageNames()

  var images []ImageResponseData

  for _, name := range names {
    images = append(images, ImageResponseData{
      Timestamp: name,
    })
  }

  ih.Templates.ExecuteTemplate(w, "images.html", ImagesPageResponseData{
    Images:   images,
    SiteInfo: ih.SiteInfo,
  })

}

func (ih *ImageResultHander) GetImageByName(w http.ResponseWriter, r *http.Request) {
  v := mux.Vars(r)

  name := v["imageName"]

  bytes, err := ih.Store.ImageByName(name)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    w.Write([]byte("Internal server error"))
  } else {
    w.Header().Set("Content-Type", "image/png")
    w.Write(bytes)
  }
}

func (ih *ImageResultHander) GetCurrentImage(w http.ResponseWriter, r *http.Request) {

  bytes, err := ih.Camera.CaptureImage(&NOW_CAMERA_SETTINGS)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    w.Write([]byte("Internal server error"))
  } else {
    w.Header().Set("Content-Type", "image/png")
    w.WriteHeader(http.StatusOK)
    w.Write(bytes)
  }
}

func (ih *ImageResultHander) GetIndex(w http.ResponseWriter, r *http.Request) {
  timelapses, err := ih.Store.GetTimelapses()
  if (err != nil) {
		timelapses = []string{}
	}
	ih.Templates.ExecuteTemplate(w, "index.html", IndexPageResponseData{
		SiteInfo: ih.SiteInfo,
		Timelapses: timelapses,
	})
}

func (ih *ImageResultHander) CreateNewTimelapse(rw http.ResponseWriter, r *http.Request) {

	interval, err := strconv.Atoi(r.FormValue("timelapse-interval"))
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Interval was not a number")
		return
	}

	http.Redirect(rw, r, ih.SiteInfo.SiteRoot+"/", http.StatusFound)

	timelapse := &TimelapseSettings{
		Name:     r.FormValue("timelapse-name"),
		Interval: time.Duration(interval) * time.Second,
		Camera:   DEFAULT_CAMERA_SETTINGS,
	}

	ih.Store.SetCurrentTimelapse(timelapse)
	ih.Camera.StartTimelapse(timelapse)
}

func handleRequests(siteInfo *SiteInfo, store *TimelapseStore, capturer *TimelapseCamera) {

  templates := template.Must(template.ParseFS(templatesFs, "templates/*.html"))

  handler := &ImageResultHander{
    SiteInfo:  siteInfo,
    Store:     store,
    Camera:    capturer,
    Templates: templates,
  }

  fs := http.FileServer(http.Dir("./src/static"))

  rootRoute := mux.NewRouter()
  rootRoute.
    PathPrefix(siteInfo.SiteRoot+"/static/").
    Handler(http.StripPrefix(siteInfo.SiteRoot+"/static/", fs))
  rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/", handler.GetImageNamePage)
  rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/latest/", handler.GetLatestImage)
  rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/now/", handler.GetCurrentImage)
  rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/{imageName}/", handler.GetImageByName)
  rootRoute.
    Path(siteInfo.SiteRoot + "/").
    Methods("GET").
    HandlerFunc(handler.GetIndex)
  rootRoute.
    Path(siteInfo.SiteRoot + "/").
    Methods("POST").
    HandlerFunc(handler.CreateNewTimelapse)

  log.Println("Starting server on port 10000")

  http.Handle("/", rootRoute)
  log.Fatal(http.ListenAndServe(":10000", nil))
}
