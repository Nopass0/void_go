package voidorm

import (
	"context"
	"testing"
)

func TestNewFromEnv(t *testing.T) {
	t.Setenv("VOIDDB_URL", "https://db.lowkey.su")
	t.Setenv("VOIDDB_TOKEN", "token-123")
	t.Setenv("VOIDDB_TIMEOUT", "42")

	client, err := NewFromEnv()
	if err != nil {
		t.Fatalf("NewFromEnv() returned error: %v", err)
	}
	if client.cfg.URL != "https://db.lowkey.su" {
		t.Fatalf("unexpected URL: %q", client.cfg.URL)
	}
	if client.Token() != "token-123" {
		t.Fatalf("unexpected token: %q", client.Token())
	}
	if client.http.Timeout.Seconds() != 42 {
		t.Fatalf("unexpected timeout: %v", client.http.Timeout)
	}
}

func TestBlobURL(t *testing.T) {
	client, err := New(Config{URL: "https://db.lowkey.su"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	col := client.DB("media").Collection("assets")

	withURL := col.BlobURL(BlobRef{
		Bucket: "media",
		Key:    "a/b.txt",
		URL:    "https://cdn.example.com/b.txt",
	})
	if withURL != "https://cdn.example.com/b.txt" {
		t.Fatalf("expected explicit URL, got %q", withURL)
	}

	fallback := col.BlobURL(BlobRef{
		Bucket: "media",
		Key:    "folder/file name.txt",
	})
	want := "https://db.lowkey.su/s3/media/folder/file%20name.txt"
	if fallback != want {
		t.Fatalf("unexpected fallback URL: got %q want %q", fallback, want)
	}
}

func TestLoginFromEnvMissingValues(t *testing.T) {
	client, err := New(Config{URL: "https://db.lowkey.su"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if _, err := client.LoginFromEnv(context.Background()); err == nil {
		t.Fatal("expected LoginFromEnv to fail without credentials")
	}
}
