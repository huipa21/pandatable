package attachments

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
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
