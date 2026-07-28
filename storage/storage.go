package storage

import (
	"errors"
	"sync"
	"time"
	"goshorty/models"
)

// ErrKeyNotFound is returned when a short code is not found
var ErrKeyNotFound = errors.New("key not found or has expired")

// StorageEntry represents a stored URL with metadata
type StorageEntry struct {
	Data      *models.URLData
	ExpiresAt time.Time
}

// Storage provides in-memory storage with TTL support
type Storage struct {
	mu   sync.RWMutex
	data map[string]*StorageEntry
	stop chan struct{}
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
	close(s.stop)
}

// Set stores a URL with TTL
func (s *Storage) Set(shortCode string, urlData *models.URLData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.data[shortCode] = &StorageEntry{
		Data:      urlData,
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
	
	return entry.Data, nil
}

// Exists checks if a short code exists and hasn't expired
func (s *Storage) Exists(shortCode string) bool {
	_, err := s.Get(shortCode)
	return err == nil
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

// IncrementVisits increments the visit count for a short code
func (s *Storage) IncrementVisits(shortCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	entry, exists := s.data[shortCode]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return ErrKeyNotFound
	}
	
	entry.Data.Visits++
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
	return entry.Data, nil
}

// GetAll returns all stored URLs (for analytics/admin)
func (s *Storage) GetAll() []*models.URLData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	results := make([]*models.URLData, 0, len(s.data))
	now := time.Now()
	
	for _, entry := range s.data {
		if !now.After(entry.ExpiresAt) {
			results = append(results, entry.Data)
		}
	}
	
	return results
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

// Stats returns storage statistics (active URLs only, ignores expired)
func (s *Storage) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalURLs := 0
	totalVisits := int64(0)
	now := time.Now()

	for _, entry := range s.data {
		if !now.After(entry.ExpiresAt) {
			totalURLs++
			totalVisits += entry.Data.Visits
		}
	}

	return map[string]interface{}{
		"total_urls":   totalURLs,
		"total_visits": totalVisits,
	}
}
