package storage

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"goshorty/models"
)

// ErrKeyNotFound is returned when a short code is not found
var ErrKeyNotFound = errors.New("key not found or has expired")

// ErrKeyExists is returned when a short code is already active.
var ErrKeyExists = errors.New("key already exists")

// StorageEntry represents a stored URL with metadata
type StorageEntry struct {
	Data      *models.URLData
	ExpiresAt time.Time
}

// Storage provides in-memory storage with TTL support
type Storage struct {
	mu       sync.RWMutex
	data     map[string]*StorageEntry
	stop     chan struct{}
	stopOnce sync.Once
}

// NewStorage creates a new in-memory storage instance
func NewStorage() *Storage {
	store := &Storage{
		data: make(map[string]*StorageEntry),
		stop: make(chan struct{}),
	}

	// Start cleanup goroutine
	go store.cleanupExpired()

	return store
}

// Stop signals the cleanup goroutine to shut down
func (s *Storage) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}

// SetIfAbsent stores a URL only when the short code is not already active.
// Expired entries may be replaced. The check and write happen under one lock.
func (s *Storage) SetIfAbsent(shortCode string, urlData *models.URLData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.data[shortCode]; exists && !time.Now().After(entry.ExpiresAt) {
		return fmt.Errorf("%w: %s", ErrKeyExists, shortCode)
	}

	s.data[shortCode] = &StorageEntry{
		Data:      cloneURLData(urlData),
		ExpiresAt: urlData.ExpiresAt,
	}
	return nil
}

// Get retrieves a URL by short code
func (s *Storage) Get(shortCode string) (*models.URLData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.data[shortCode]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, ErrKeyNotFound
	}

	return cloneURLData(entry.Data), nil
}

// Delete removes a short code from storage
func (s *Storage) Delete(shortCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[shortCode]; !exists {
		return ErrKeyNotFound
	}

	delete(s.data, shortCode)
	return nil
}

// GetAndIncrement atomically retrieves URL data and increments visit count
func (s *Storage) GetAndIncrement(shortCode string) (*models.URLData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[shortCode]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return nil, ErrKeyNotFound
	}

	entry.Data.Visits++
	return cloneURLData(entry.Data), nil
}

// GetAllFor returns all active URLs visible to the caller.
func (s *Storage) GetAllFor(userID, role string) ([]*models.URLData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*models.URLData, 0, len(s.data))
	now := time.Now()

	for _, entry := range s.data {
		if now.After(entry.ExpiresAt) {
			continue
		}
		if role != models.RoleAdmin && entry.Data.CreatedBy != userID {
			continue
		}
		results = append(results, cloneURLData(entry.Data))
	}

	return results, nil
}

// cleanupExpired removes expired entries periodically
func (s *Storage) cleanupExpired() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()

			for key, entry := range s.data {
				if now.After(entry.ExpiresAt) {
					delete(s.data, key)
				}
			}

			s.mu.Unlock()
		}
	}
}

// StatsFor returns statistics for active URLs visible to a caller.
func (s *Storage) StatsFor(userID, role string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalURLs := 0
	totalVisits := int64(0)
	now := time.Now()

	for _, entry := range s.data {
		if now.After(entry.ExpiresAt) {
			continue
		}
		if role != models.RoleAdmin && entry.Data.CreatedBy != userID {
			continue
		}
		totalURLs++
		totalVisits += entry.Data.Visits
	}

	return map[string]interface{}{
		"total_urls":   totalURLs,
		"total_visits": totalVisits,
	}, nil
}

func cloneURLData(data *models.URLData) *models.URLData {
	if data == nil {
		return nil
	}
	cloned := *data
	return &cloned
}
