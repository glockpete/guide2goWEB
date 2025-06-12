package main

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestIsValidImageID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"abc-123_foo.bar", true},
		{"../etc/passwd", false},
		{"image@id", false},
		{"image.png", true},
		{"image space", false},
	}
	for _, c := range cases {
		if got := isValidImageID(c.id); got != c.valid {
			t.Errorf("isValidImageID(%q) = %v, want %v", c.id, got, c.valid)
		}
	}
}

func TestValidateImagePath(t *testing.T) {
	base := "/tmp/images"
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"foo.png", false},
		{"../etc/passwd", true},
		{"subdir/bar.jpg", false},
	}
	for _, c := range cases {
		err := validateImagePath(base, c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("validateImagePath(%q) error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestBufferPoolReuse(t *testing.T) {
	buf1 := bufferPool.Get().([]byte)
	bufferPool.Put(buf1)
	buf2 := bufferPool.Get().([]byte)
	if &buf1[0] != &buf2[0] {
		t.Error("Buffer pool did not reuse buffer")
	}
	bufferPool.Put(buf2)
}

// Note: Full integration tests for GetImageUrl would require refactoring for dependency injection of httpClient and file system.

// Placeholder for GetImageUrl tests (requires refactoring for testability)
func TestGetImageUrl(t *testing.T) {
	t.Skip("GetImageUrl requires dependency injection for HTTP and file system to be testable")
}

func TestCacheInitAndCleanUp(t *testing.T) {
	c := &cache{}
	app := &App{Logger: logrus.New(), Config: config{}}
	c.Init()
	if c.Channel == nil || c.Program == nil || c.Metadata == nil || c.Schedule == nil {
		t.Error("Cache maps not initialized")
	}
	c.CleanUp(app)
	// Should not panic or error
}

func TestCacheOpenAndSave(t *testing.T) {
	c := &cache{}
	app := &App{Logger: logrus.New(), Config: config{}}
	c.File = "testcache"
	defer os.Remove("testcache.yaml")
	if err := c.Save(app); err != nil {
		t.Errorf("Failed to save cache: %v", err)
	}
	if err := c.Open(app); err != nil {
		t.Errorf("Failed to open cache: %v", err)
	}
}
