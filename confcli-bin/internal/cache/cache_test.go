package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheOperations(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Create a cache instance
	cache, err := NewCache(5) // 5 minute TTL
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	
	// Override cache dir for testing
	cache.cacheDir = tempDir
	
	// Test Set and Get operations
	testKey := "test_key"
	testValue := "test_value"
	
	// Set a value
	if err := cache.Set(testKey, testValue); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}
	
	// Get the value
	value, found, err := cache.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	
	if !found {
		t.Error("Expected value to be found in cache")
	}
	
	if value != testValue {
		t.Errorf("Expected value to be '%s', got '%v'", testValue, value)
	}
	
	// Test Delete operation
	if err := cache.Delete(testKey); err != nil {
		t.Fatalf("Failed to delete value: %v", err)
	}
	
	// Verify deletion
	_, found, err = cache.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get value after deletion: %v", err)
	}
	
	if found {
		t.Error("Expected value to not be found after deletion")
	}
}

func TestCacheExpiration(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Create a cache with a short TTL
	cache, err := NewCache(0) // 0 minute TTL (immediate expiration)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	
	// Override cache dir for testing
	cache.cacheDir = tempDir
	
	testKey := "expiring_key"
	testValue := "expiring_value"
	
	// Set a value
	if err := cache.Set(testKey, testValue); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}
	
	// Manually modify the file's modification time to simulate expiration
	cacheFile := filepath.Join(cache.cacheDir, cache.generateKey(testKey)+".json")
	if err := os.Chtimes(cacheFile, time.Now().Add(-1*time.Hour), time.Now().Add(-1*time.Hour)); err != nil {
		t.Fatalf("Failed to change file time: %v", err)
	}
	
	// Try to get the expired value (should not be found)
	_, found, err := cache.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}
	
	if found {
		t.Error("Expected expired value to not be found in cache")
	}
}

func TestGetWithDefault(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Create a cache instance
	cache, err := NewCache(5) // 5 minute TTL
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	
	// Override cache dir for testing
	cache.cacheDir = tempDir
	
	testKey := "default_key"
	defaultValue := "default_value"
	
	// Define a function that returns the default value
	fn := func() (interface{}, error) {
		return defaultValue, nil
	}
	
	// Get with default (value should be computed and cached)
	value, err := cache.GetWithDefault(testKey, fn)
	if err != nil {
		t.Fatalf("Failed to get with default: %v", err)
	}
	
	if value != defaultValue {
		t.Errorf("Expected value to be '%s', got '%v'", defaultValue, value)
	}
	
	// Verify the value is now in cache
	cachedValue, found, err := cache.Get(testKey)
	if err != nil {
		t.Fatalf("Failed to get cached value: %v", err)
	}
	
	if !found {
		t.Error("Expected value to be found in cache after GetWithDefault")
	}
	
	if cachedValue != defaultValue {
		t.Errorf("Expected cached value to be '%s', got '%v'", defaultValue, cachedValue)
	}
}