package attachments

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pandatable/internal/apperr"
)

func TestCleanNameAndMIMEPolicy(t *testing.T) {
	name, err := cleanName(`C:\upload\résumé.pdf`)
	if err != nil || name != "résumé.pdf" {
		t.Fatalf("cleanName = %q, %v", name, err)
	}
	for _, value := range []string{"", "..", "bad\rname", "too-long" + string(make([]byte, maxStoredFileName+1))} {
		if _, err := cleanName(value); err == nil {
			t.Errorf("cleanName(%q) succeeded", value)
		}
	}
	for _, test := range []struct {
		mime, accept string
		want         bool
	}{{"image/png", "image/*, application/pdf", true}, {"application/pdf", "image/*", false},
		{"image/svg+xml", "image/*", true}} {
		if got := accepts(test.mime, test.accept); got != test.want {
			t.Errorf("accepts(%q, %q) = %v", test.mime, test.accept, got)
		}
	}
	if safeImage("image/svg+xml") || !safeImage("image/webp") {
		t.Fatal("inline image allowlist is incorrect")
	}
}

func TestSniffWebPAndBoundedThumbnail(t *testing.T) {
	directory := t.TempDir()
	webpPath := filepath.Join(directory, "fake.webp")
	if err := os.WriteFile(webpPath, []byte("RIFF\x04\x00\x00\x00WEBP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := sniffFile(webpPath); err != nil || got != "image/webp" {
		t.Fatalf("sniffFile(WebP) = %q, %v", got, err)
	}

	imagePath := filepath.Join(directory, "image.png")
	thumbnailPath := filepath.Join(directory, "thumb.png")
	var imageBytes = image.NewRGBA(image.Rect(0, 0, 512, 300))
	output, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(output, imageBytes); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if err := makeThumbnail(imagePath, thumbnailPath, "image/png", 1); err != nil {
		t.Fatal(err)
	}
	thumbnail, err := os.Open(thumbnailPath)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := png.DecodeConfig(thumbnail)
	_ = thumbnail.Close()
	if err != nil || configuration.Width > thumbnailSize || configuration.Height > thumbnailSize {
		t.Fatalf("thumbnail dimensions=%dx%d err=%v", configuration.Width, configuration.Height, err)
	}

	largePath := filepath.Join(directory, "large.png")
	output, err = os.Create(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(output, image.NewRGBA(image.Rect(0, 0, 1200, 1000))); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	err = makeThumbnail(largePath, filepath.Join(directory, "large-thumb.png"), "image/png", 1)
	var appError *apperr.Error
	if !errors.As(err, &appError) || appError.Code != "ERR_IMAGE_TOO_LARGE" {
		t.Fatalf("oversized image error = %v", err)
	}
}

func TestCloneStoredFileSharesDataAndRemovesIndependently(t *testing.T) {
	root := t.TempDir()
	service := &Service{root: root}
	sourceID := "01900000-0000-7000-8000-000000000001"
	newID := "01900000-0000-7000-8000-000000000002"
	directory := filepath.Join(root, "2025", "01")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, sourceID), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, sourceID+".thumb.png"), []byte("thumb"), 0o600); err != nil {
		t.Fatal(err)
	}
	stored := "files/2025/01/" + sourceID
	for _, bad := range []struct{ source, path, target string }{
		{sourceID, stored, sourceID},
		{sourceID, "files/2025/01/../" + sourceID, newID},
		{sourceID, "files/2025/01/" + newID, newID},
		{"not-a-uuid", stored, newID},
	} {
		if _, err := service.CloneStoredFile(context.Background(), bad.source, bad.path, false, bad.target); err == nil {
			t.Errorf("CloneStoredFile(%+v) succeeded", bad)
		}
	}
	missing := "01900000-0000-7000-8000-000000000003"
	if _, err := service.CloneStoredFile(context.Background(), missing, "files/2025/01/"+missing, false,
		newID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing clone source error = %v", err)
	}
	path, err := service.CloneStoredFile(context.Background(), sourceID, stored, true, newID)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[0] != "files" || parts[3] != newID {
		t.Fatalf("clone path = %q", path)
	}
	clone := filepath.Join(root, parts[1], parts[2], newID)
	if _, err := service.CloneStoredFile(context.Background(), sourceID, stored, true, newID); err == nil {
		t.Fatal("clone overwrote an existing target")
	}
	if err := service.RemovePurgedFile(context.Background(), sourceID, stored, true); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{clone: "payload", clone + ".thumb.png": "thumb"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("clone %s = %q, %v", name, data, err)
		}
	}
	if err := service.RemovePurgedFile(context.Background(), newID, path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed clone still exists: %v", err)
	}
}
