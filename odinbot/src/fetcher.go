package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type OdinFetcher struct {
	Store         *OdinBotStore
	TargetURL     string
	FetchInterval int
}

func (f *OdinFetcher) Start() {
	log.Printf("Starting Odin fetcher, fetching %s every %d seconds", f.TargetURL, f.FetchInterval)

	// Do an initial fetch immediately
	f.fetch()

	ticker := time.NewTicker(time.Duration(f.FetchInterval) * time.Second)
	for range ticker.C {
		f.fetch()
	}
}

func (f *OdinFetcher) fetch() {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, f.TargetURL, nil)
	if err != nil {
		log.Printf("Error creating request for %s: %v", f.TargetURL, err)
		return
	}
	req.Header.Set("User-Agent", "OdinBot https://home.ticklethepanda.co.uk/odinbot/")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error fetching %s: %v", f.TargetURL, err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}

	imageURL, err := extractCatImageURL(body, resp.Request.URL)
	if err != nil {
		log.Printf("Unable to determine cat image URL: %v", err)
	}

	record := &FetchRecord{
		Time:     time.Now(),
		ImageURL: imageURL,
	}

	if err := f.Store.Add(record); err != nil {
		log.Printf("Error storing fetch record: %v", err)
		return
	}

	if imageURL != "" {
		log.Printf("Successfully fetched %s (status: %d, image: %s)", f.TargetURL, resp.StatusCode, imageURL)
		return
	}

	log.Printf("Successfully fetched %s (status: %d) but no image was recorded", f.TargetURL, resp.StatusCode)
}

func extractCatImageURL(body []byte, base *url.URL) (string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	src := findFirstImageSrc(doc)
	if src == "" {
		return "", errors.New("no <img> src found in page")
	}

	return resolveImageURL(src, base)
}

func findFirstImageSrc(node *html.Node) string {
	if node == nil {
		return ""
	}

	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "img") {
		for _, attr := range node.Attr {
			if strings.EqualFold(attr.Key, "src") {
				return attr.Val
			}
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if src := findFirstImageSrc(child); src != "" {
			return src
		}
	}

	return ""
}

func resolveImageURL(src string, base *url.URL) (string, error) {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" {
		return "", errors.New("empty image src")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}

	if parsed.IsAbs() {
		return parsed.String(), nil
	}

	if base == nil {
		return "", errors.New("cannot resolve relative image src without base URL")
	}

	return base.ResolveReference(parsed).String(), nil
}
