package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type FloofMajestyStore struct {
	File   string
	mu     sync.RWMutex
	scores map[string]float64
}

func NewFloofMajestyStore(file string) (*FloofMajestyStore, error) {
	store := &FloofMajestyStore{
		File:   file,
		scores: make(map[string]float64),
	}

	if err := store.ensureDir(); err != nil {
		return nil, err
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *FloofMajestyStore) ensureDir() error {
	dir := filepath.Dir(s.File)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func (s *FloofMajestyStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.File, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReader(f))
	reader.FieldsPerRecord = -1

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if len(record) < 2 {
			continue
		}
		imageURL := strings.TrimSpace(record[0])
		score, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
		if err != nil {
			continue
		}
		s.scores[imageURL] = score
	}

	return nil
}

func (s *FloofMajestyStore) Get(imageURL string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	score, ok := s.scores[imageURL]
	return score, ok
}

func (s *FloofMajestyStore) Set(imageURL string, score float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.scores[imageURL]; ok {
		if existing == score {
			return nil
		}
	}

	s.scores[imageURL] = score
	return s.saveLocked()
}

func (s *FloofMajestyStore) saveLocked() error {
	if err := s.ensureDir(); err != nil {
		return err
	}

	f, err := os.Create(s.File)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	keys := make([]string, 0, len(s.scores))
	for key := range s.scores {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		line := []string{key, fmt.Sprintf("%.6f", s.scores[key])}
		if err := writer.Write(line); err != nil {
			return err
		}
	}

	return writer.Error()
}

func (s *FloofMajestyStore) TopScore() (string, float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var topURL string
	var topScore float64
	var found bool

	for url, score := range s.scores {
		if !found || score > topScore {
			topURL = url
			topScore = score
			found = true
		}
	}

	return topURL, topScore, found
}
