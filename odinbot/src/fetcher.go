package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type OdinFetcher struct {
	Store          *OdinBotStore
	TargetURL      string
	FetchInterval  int
	FloofEvaluator *FloofMajestyEvaluator
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodGet, f.TargetURL, nil)
	if err != nil {
		log.Printf("Error creating request for %s: %v", f.TargetURL, err)
		return
	}
	req.Header.Set("User-Agent", "OdinBot https://home.ticklethepanda.co.uk/odinbot/")

	resp, err := client.Do(req)
	if err != nil {
		reason := fmt.Sprintf("request failed: %v", err)
		log.Printf("Error fetching %s: %s", f.TargetURL, reason)
		f.recordFailure(reason)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		resolved := resolveRedirectLocation(location, resp.Request.URL)
		reason := "redirected without a location"
		if resolved != "" {
			reason = "redirected to " + resolved
		}
		f.recordFailure(reason)
		log.Printf("Redirect detected while fetching %s (status: %d, location: %s)", f.TargetURL, resp.StatusCode, resolved)
		return
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		reason := fmt.Sprintf("unexpected status %d", resp.StatusCode)
		f.recordFailure(reason)
		log.Printf("Unexpected status while fetching %s: %s", f.TargetURL, reason)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		reason := fmt.Sprintf("error reading response body: %v", err)
		log.Println(reason)
		f.recordFailure(reason)
		return
	}

	imageURL, err := extractCatImageURL(body, resp.Request.URL)
	if err != nil {
		reason := fmt.Sprintf("unable to determine cat image URL: %v", err)
		log.Println(reason)
		f.recordFailure(reason)
		return
	}

	if imageURL == "" {
		reason := "page loaded but no image URL was found"
		log.Println(reason)
		f.recordFailure(reason)
		return
	}

	if err := f.Store.Add(&FetchRecord{Time: time.Now(), ImageURL: imageURL}); err != nil {
		log.Printf("Error storing fetch record: %v", err)
		return
	}

	if f.FloofEvaluator != nil {
		if score, err := f.FloofEvaluator.Score(imageURL); err != nil {
			log.Printf("Error computing Floof Majesty Index for %s: %v", imageURL, err)
		} else {
			log.Printf("Floof Majesty Index for %s: %.3f", imageURL, score)
		}
	}

	log.Printf("Successfully fetched %s (status: %d, image: %s)", f.TargetURL, resp.StatusCode, imageURL)
}

func (f *OdinFetcher) recordFailure(reason string) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		trimmed = "unknown error"
	}
	record := &FetchRecord{
		Time:          time.Now(),
		FailureReason: trimmed,
	}
	if err := f.Store.Add(record); err != nil {
		log.Printf("Error storing failure record: %v", err)
	}
}

func resolveRedirectLocation(location string, base *url.URL) string {
	trimmed := strings.TrimSpace(location)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	if base == nil {
		return parsed.String()
	}
	return base.ResolveReference(parsed).String()
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
