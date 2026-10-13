package storage

import (
	"bytes"
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"testing"
	"time"
)

func TestS3Integration(t *testing.T) {
	secret := os.Getenv("ISLET_TEST_S3_SECRET")
	if secret == "" {
		t.Skip("ISLET_TEST_S3_SECRET is required for the isolated MinIO service")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := minio.New("127.0.0.1:29000", &minio.Options{Creds: credentials.NewStaticV4("isletreview", secret, ""), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	exists, err := client.BucketExists(ctx, "islet-review")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, "islet-review", minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
			t.Fatal(err)
		}
	}
	d := NewS3(S3Config{Endpoint: "http://127.0.0.1:29000", Region: "us-east-1", Bucket: "islet-review", AccessKey: "isletreview", SecretKey: secret, PathStyle: true}, func() string { return "http://127.0.0.1:28090" })
	d.SetProxy(true)
	if err := d.Check(ctx); err != nil {
		t.Fatal(err)
	}
	data := buildProbePNG()
	key := "integration-" + time.Now().Format("20060102-150405.000000") + ".png"
	got, err := d.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:28090/i/"+key {
		t.Fatal("unexpected image URL")
	}
	r, size, err := d.Read(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(r)
	r.Close()
	if err != nil || size != int64(len(data)) || !bytes.Equal(read, data) {
		t.Fatal("stored object differs")
	}
	if err := d.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Read(ctx, key); err == nil {
		t.Fatal("deleted object remains readable")
	}
}
