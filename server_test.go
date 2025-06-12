package main

import (
	"testing"
)

func TestIsValidImageID(t *testing.T) {
	cases := []struct {
		id     string
		valid  bool
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

// Placeholder for HTTP handler tests
func TestProxyImagesHandler(t *testing.T) {
	t.Skip("ProxyImages handler test requires HTTP test server setup")
} 