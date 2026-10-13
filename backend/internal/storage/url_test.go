package storage

import "testing"

func TestS3ApplicationURL(t *testing.T) {
	d := NewS3(S3Config{Endpoint: "https://objects.example", Bucket: "photos"}, func() string { return "https://images.example" })
	d.SetProxy(true)
	if d.Access() != AccessProxy || d.publicURL("a.png") != "https://images.example/i/a.png" {
		t.Fatal("private objects must be accessed through the application")
	}
	d = NewS3(S3Config{CDN: "https://cdn.example", Prefix: "photos"}, nil)
	if d.Access() != AccessRedirect || d.publicURL("a.png") != "https://cdn.example/photos/a.png" {
		t.Fatal("CDN URL must retain prefix")
	}
	d.SetProxy(true)
	if d.Access() != AccessProxy || d.publicURL("a.png") != "/i/a.png" {
		t.Fatal("explicit proxy mode must override CDN")
	}
}
