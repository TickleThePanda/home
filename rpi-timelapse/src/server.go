package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type ImageResultHander struct {
	SiteInfo  *SiteInfo
	Store     *TimelapseStore
	Camera    *TimelapseCamera
	Templates *Templates
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

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)

	ih.Store.LatestImage(w)
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

	ih.Templates.Images.Execute(w, ImagesPageResponseData{
		Images:   images,
		SiteInfo: ih.SiteInfo,
	})

}

func (ih *ImageResultHander) GetImageByName(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)

	name := v["imageName"]

	w.Header().Set("Content-Type", "image/png")

	w.WriteHeader(http.StatusOK)

	ih.Store.ImageByName(name, w)
}

func (ih *ImageResultHander) GetCurrentImage(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "image/png")

	w.WriteHeader(http.StatusOK)

	ih.Camera.CaptureImage(&NOW_CAMERA_SETTINGS, w)
}

func handleRequests(siteInfo *SiteInfo, store *TimelapseStore, capturer *TimelapseCamera) {

	templates := &Templates{
		Index:  template.Must(template.ParseFiles("./src/templates/index.html")),
		Images: template.Must(template.ParseFiles("./src/templates/images.html")),
	}

	handler := &ImageResultHander{
		SiteInfo:  siteInfo,
		Store:     store,
		Camera:    capturer,
		Templates: templates,
	}

	rootRoute := mux.NewRouter()
	rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/", handler.GetImageNamePage)
	rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/latest/", handler.GetLatestImage)
	rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/now/", handler.GetCurrentImage)
	rootRoute.HandleFunc(siteInfo.SiteRoot+"/images/{imageName}/", handler.GetImageByName)
	rootRoute.
		Path(siteInfo.SiteRoot + "/").
		Methods("GET").
		HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			templates.Index.Execute(rw, IndexPageResponseData{
				SiteInfo: siteInfo,
			})
		})
	rootRoute.
		Path(siteInfo.SiteRoot + "/").
		Methods("POST").
		HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {

			interval, err := strconv.Atoi(r.FormValue("timelapse-interval"))
			if err != nil {
				rw.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(rw, "Interval was not a number")
				return
			}

			http.Redirect(rw, r, siteInfo.SiteRoot+"/", http.StatusFound)

			timelapse := &TimelapseSettings{
				Name:     r.FormValue("timelapse-name"),
				Interval: time.Duration(interval) * time.Second,
				Camera:   DEFAULT_CAMERA_SETTINGS,
			}

			store.SetCurrentTimelapse(timelapse)
			capturer.StartTimelapse(timelapse)
		})

	log.Println("Starting server on port 10000")

	http.Handle("/", rootRoute)
	log.Fatal(http.ListenAndServe(":10000", nil))
}
