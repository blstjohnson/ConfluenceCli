package cache

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"confcli/internal/config"
)

// Cache represents a disk-based cache
type Cache struct {
	cacheDir string
	ttl      time.Duration
}

// NewCache creates a new cache instance
func NewCache(ttlMinutes int) (*Cache, error) {
	cacheDir, err := config.GetCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache directory: %w", err)
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{
		cacheDir: cacheDir,
		ttl:      time.Duration(ttlMinutes) * time.Minute,
	}, nil
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool, error) {
	cacheKey := c.generateKey(key)
	cacheFile := filepath.Join(c.cacheDir, cacheKey+".json")

	// Check if file exists
	info, err := os.Stat(cacheFile)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	// Check if cache is expired
	if time.Since(info.ModTime()) > c.ttl {
		// Remove expired cache file
		os.Remove(cacheFile)
		return nil, false, nil
	}

	// Read cached data
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, false, err
	}

	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, err
	}

	return result, true, nil
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value interface{}) error {
	cacheKey := c.generateKey(key)
	cacheFile := filepath.Join(c.cacheDir, cacheKey+".json")

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return os.WriteFile(cacheFile, data, 0644)
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) error {
	cacheKey := c.generateKey(key)
	cacheFile := filepath.Join(c.cacheDir, cacheKey+".json")

	return os.Remove(cacheFile)
}

// Clear removes all cached values
func (c *Cache) Clear() error {
	files, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(c.cacheDir, file.Name())); err != nil {
			// Log error but continue with other files
			continue
		}
	}

	return nil
}

// generateKey creates a unique key based on the input
func (c *Cache) generateKey(input string) string {
	hash := md5.Sum([]byte(input))
	return hex.EncodeToString(hash[:])
}

// GetWithDefault retrieves a value from the cache or executes a function to get the value
func (c *Cache) GetWithDefault(key string, fn func() (interface{}, error)) (interface{}, error) {
	// Try to get from cache
	value, found, err := c.Get(key)
	if err != nil {
		// If there's an error getting from cache, just execute the function
		return fn()
	}

	if found {
		return value, nil
	}

	// Value not in cache, execute the function
	value, err = fn()
	if err != nil {
		return nil, err
	}

	// Store in cache
	if err := c.Set(key, value); err != nil {
		// Log error but return the value anyway
	}

	return value, nil
}