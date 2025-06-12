package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gorilla/mux"
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

func TestHealthCheckHandler(t *testing.T) {
	app := newApp()
	req := httptest.NewRequest("GET", "/health", nil)
	rw := httptest.NewRecorder()
	app.healthCheck(rw, req)
	if rw.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rw.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", resp["status"])
	}
}

// Placeholder for HTTP handler tests
func TestProxyImagesHandler(t *testing.T) {
	t.Skip("ProxyImages handler test requires HTTP test server setup")
}

func TestRunHandler(t *testing.T) {
	app := newApp()
	req := httptest.NewRequest("GET", "/run", nil)
	rw := httptest.NewRecorder()
	app.run(rw, req)
	if rw.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rw.Code)
	}
	if rw.Body.String() != "Grabbing EPG" {
		t.Errorf("Expected 'Grabbing EPG', got %q", rw.Body.String())
	}
}

func TestProxyImagesHandler_InvalidID(t *testing.T) {
	app := newApp()
	req := httptest.NewRequest("GET", "/images/../../etc/passwd", nil)
	rw := httptest.NewRecorder()
	vars := map[string]string{"id": "../../etc/passwd"}
	req = mux.SetURLVars(req, vars)
	app.proxyImages(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %d", rw.Code)
	}
} 