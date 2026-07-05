package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 1x1 PNG.
var onePixelPNG = mustB64("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC")

func mustB64(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestFetchImage_SavesToReferences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(onePixelPNG)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tool := &FetchImageTool{Config: ImageToolsConfig{ReferencesDir: dir}}
	res, err := tool.Execute(context.Background(), map[string]any{"url": srv.URL + "/art/goblin.png"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	path, _ := res.Output["path"].(string)
	if path == "" {
		t.Fatal("no path returned")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("expected .png, got %s", path)
	}
}

func TestFetchImage_MissingURL(t *testing.T) {
	tool := &FetchImageTool{Config: ImageToolsConfig{ReferencesDir: t.TempDir()}}
	res, _ := tool.Execute(context.Background(), map[string]any{})
	if res.Success {
		t.Fatal("expected failure for missing url")
	}
}

func TestGenerateImage_NotConfigured(t *testing.T) {
	// No endpoint set and no env → graceful "not configured".
	t.Setenv("PRISM_IMAGEGEN_URL", "")
	tool := &GenerateImageTool{Config: ImageToolsConfig{ReferencesDir: t.TempDir()}}
	res, _ := tool.Execute(context.Background(), map[string]any{"prompt": "a goblin"})
	if res.Success {
		t.Fatal("expected not-configured failure")
	}
	if !strings.Contains(res.Error, "not configured") {
		t.Errorf("expected 'not configured', got: %s", res.Error)
	}
}

func TestGenerateImage_RawImageResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(onePixelPNG)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tool := &GenerateImageTool{Config: ImageToolsConfig{ReferencesDir: dir, ImageGenEndpoint: srv.URL}}
	res, err := tool.Execute(context.Background(), map[string]any{"prompt": "a stylized goblin warrior"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if p, _ := res.Output["path"].(string); p == "" {
		t.Fatal("no path returned")
	}
}

func TestGenerateImage_JSONBase64Response(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(onePixelPNG)}},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	tool := &GenerateImageTool{Config: ImageToolsConfig{ReferencesDir: dir, ImageGenEndpoint: srv.URL}}
	res, err := tool.Execute(context.Background(), map[string]any{"prompt": "concept art"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	path, _ := res.Output["path"].(string)
	got, _ := os.ReadFile(path)
	if len(got) != len(onePixelPNG) {
		t.Errorf("decoded image size mismatch: got %d want %d", len(got), len(onePixelPNG))
	}
}

func TestAnalyzeImage_VisionResponse(t *testing.T) {
	var gotImages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Images []string `json:"images"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) > 0 {
			gotImages = len(body.Messages[0].Images)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"content": "A green goblin warrior, painterly style, earthy palette."},
		})
	}))
	defer srv.Close()

	// Local image file to analyze.
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "ref.png")
	os.WriteFile(imgPath, onePixelPNG, 0o644)

	tool := &AnalyzeImageTool{Config: ImageToolsConfig{VisionBaseURL: srv.URL, VisionModel: "llama3.2-vision:11b"}}
	res, err := tool.Execute(context.Background(), map[string]any{"image": imgPath})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if gotImages != 1 {
		t.Errorf("expected 1 base64 image sent, got %d", gotImages)
	}
	desc, _ := res.Output["description"].(string)
	if !strings.Contains(desc, "goblin") {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestAnalyzeImage_MissingImage(t *testing.T) {
	tool := &AnalyzeImageTool{Config: ImageToolsConfig{}}
	res, _ := tool.Execute(context.Background(), map[string]any{})
	if res.Success {
		t.Fatal("expected failure for missing image")
	}
}

func TestRegisterImageTools(t *testing.T) {
	reg := NewRegistry()
	RegisterImageTools(reg, ImageToolsConfig{})
	for _, name := range []string{"fetch_image", "generate_image", "analyze_image"} {
		if _, err := reg.Resolve(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
	}
}

func TestImageToolsConfig_Defaults(t *testing.T) {
	t.Setenv("PRISM_IMAGE_DIR", "")
	t.Setenv("PRISM_VISION_MODEL", "")
	t.Setenv("PRISM_VISION_URL", "")
	t.Setenv("OLLAMA_BASE_URL", "")
	c := ImageToolsConfig{}
	if c.referencesDir() != defaultReferencesDir {
		t.Errorf("referencesDir default = %q", c.referencesDir())
	}
	if c.visionModel() != defaultVisionModel {
		t.Errorf("visionModel default = %q", c.visionModel())
	}
	if c.visionBaseURL() != "http://localhost:11434" {
		t.Errorf("visionBaseURL default = %q", c.visionBaseURL())
	}
}
