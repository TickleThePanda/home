package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

type ImageResultHander struct {
	Store    *TimelapseStore
	Capturer *ImageCapturer
}

func (ih *ImageResultHander) GetLatestImage(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)

	ih.Store.LatestImage(w)
}

func (ih *ImageResultHander) GetImageNamePage(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	names, _ := ih.Store.ImageNames()
	var builder strings.Builder

	builder.WriteString("<ul>")
	builder.WriteString("<li><a href=\"latest/\">latest</a>")
	for _, n := range names {
		fmt.Fprintf(&builder, "<li><a href=\"%v/\">%v</a>", n, n)
	}
	builder.WriteString("</ul>")

	w.Write([]byte(builder.String()))

}

func (ih *ImageResultHander) GetImageByName(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)

	name := v["imageName"]

	log.Println(r.URL)
	w.Header().Set("Content-Type", "image/png")

	w.WriteHeader(http.StatusOK)

	ih.Store.ImageByName(name, w)
}

func handleRequests(siteRoot string, store *TimelapseStore, capturer *ImageCapturer) {

	handler := &ImageResultHander{
		Store:    store,
		Capturer: capturer,
	}

	rootRoute := mux.NewRouter()
	rootRoute.HandleFunc(siteRoot+"/images/", handler.GetImageNamePage)
	rootRoute.HandleFunc(siteRoot+"/images/latest/", handler.GetLatestImage)
	rootRoute.HandleFunc(siteRoot+"/images/{imageName}/", handler.GetImageByName)

	log.Println("Starting server on port 10000")

	http.Handle("/", rootRoute)
	log.Fatal(http.ListenAndServe(":10000", nil))
}
