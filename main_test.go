package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newTestServer creates a test server and S3 client pointing to it
func newTestServer(t *testing.T) (*httptest.Server, *s3.Client, func()) {
	t.Helper()

	// Create temp data dir
	dataDir := t.TempDir()

	store, err := NewObjectStore(dataDir)
	if err != nil {
		t.Fatalf("failed to create object store: %v", err)
	}

	server := httptest.NewServer(store)

	// Create S3 client pointing to test server
	cfg, _ := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		config.WithRegion("us-east-1"),
	)

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
	})

	cleanup := func() {
		server.Close()
	}

	return server, client, cleanup
}

func TestCreateBucket(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "test-bucket"

	// Create bucket
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}

	// Verify bucket exists with HeadBucket
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("HeadBucket failed after create: %v", err)
	}
}

func TestListBuckets(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketNames := []string{"bucket-a", "bucket-b", "bucket-c"}

	// Create multiple buckets
	for _, name := range bucketNames {
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(name),
		})
		if err != nil {
			t.Fatalf("CreateBucket %s failed: %v", name, err)
		}
	}

	// List buckets
	resp, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		t.Fatalf("ListBuckets failed: %v", err)
	}

	if len(resp.Buckets) != len(bucketNames) {
		t.Errorf("expected %d buckets, got %d", len(bucketNames), len(resp.Buckets))
	}
}

func TestDeleteBucket(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "delete-me"

	// Create bucket
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}

	// Delete bucket
	_, err = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("DeleteBucket failed: %v", err)
	}

	// Verify bucket no longer exists
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		t.Error("expected HeadBucket to fail after delete")
	}
}

func TestPutAndGetObject(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "object-bucket"
	objectKey := "test-file.txt"
	objectContent := []byte("Hello, S3-compatible world!")

	// Create bucket first
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("CreateBucket failed: %v", err)
	}

	// Put object
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(objectContent),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		t.Fatalf("PutObject failed: %v", err)
	}

	// Get object
	getResp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("GetObject failed: %v", err)
	}
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	if string(body) != string(objectContent) {
		t.Errorf("content mismatch: got %q, want %q", string(body), string(objectContent))
	}
}

func TestHeadObject(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "head-bucket"
	objectKey := "metadata-file.txt"
	objectContent := []byte("Check my headers!")

	// Create bucket
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	// Put object
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(objectContent),
		ContentType: aws.String("text/plain"),
	})

	// Head object
	headResp, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("HeadObject failed: %v", err)
	}

	if *headResp.ContentLength != int64(len(objectContent)) {
		t.Errorf("content length mismatch: got %d, want %d", *headResp.ContentLength, len(objectContent))
	}
}

func TestDeleteObject(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "delete-obj-bucket"
	objectKey := "to-delete.txt"

	// Setup
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader([]byte("delete me")),
	})

	// Delete object
	_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("DeleteObject failed: %v", err)
	}

	// Verify object is gone
	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err == nil {
		t.Error("expected HeadObject to fail after delete")
	}
}

func TestListObjects(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "list-bucket"

	// Create bucket
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	// Put multiple objects
	objects := []string{"file1.txt", "file2.txt", "docs/readme.md"}
	for _, key := range objects {
		_, _ = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
			Body:   bytes.NewReader([]byte("content")),
		})
	}

	// List objects
	listResp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 failed: %v", err)
	}

	if len(listResp.Contents) != len(objects) {
		t.Errorf("expected %d objects, got %d", len(objects), len(listResp.Contents))
	}
}

func TestListObjectsWithPrefix(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "prefix-bucket"

	// Create bucket
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	// Put objects with different prefixes
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("images/photo1.jpg"),
		Body:   bytes.NewReader([]byte("img1")),
	})
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("images/photo2.jpg"),
		Body:   bytes.NewReader([]byte("img2")),
	})
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("docs/readme.md"),
		Body:   bytes.NewReader([]byte("readme")),
	})

	// List only images
	listResp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
		Prefix: aws.String("images/"),
	})
	if err != nil {
		t.Fatalf("ListObjectsV2 with prefix failed: %v", err)
	}

	if len(listResp.Contents) != 2 {
		t.Errorf("expected 2 objects with images/ prefix, got %d", len(listResp.Contents))
	}
}

func TestBucketAlreadyExists(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "duplicate-bucket"

	// Create bucket
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("first CreateBucket failed: %v", err)
	}

	// Try to create same bucket again
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		t.Error("expected error when creating duplicate bucket")
	}
}

func TestDeleteNonEmptyBucket(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "non-empty-bucket"

	// Create bucket with object
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	_, _ = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String("file.txt"),
		Body:   bytes.NewReader([]byte("data")),
	})

	// Try to delete non-empty bucket
	_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		t.Error("expected error when deleting non-empty bucket")
	}
}

func TestNestedObjectPaths(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "nested-bucket"
	nestedKey := "a/b/c/d/deep-file.txt"
	content := []byte("deeply nested content")

	// Create bucket
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	// Put deeply nested object
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(nestedKey),
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("PutObject for nested path failed: %v", err)
	}

	// Get deeply nested object
	getResp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(nestedKey),
	})
	if err != nil {
		t.Fatalf("GetObject for nested path failed: %v", err)
	}
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	if string(body) != string(content) {
		t.Errorf("nested content mismatch")
	}
}

func TestLargeObject(t *testing.T) {
	_, client, cleanup := newTestServer(t)
	defer cleanup()

	ctx := context.Background()
	bucketName := "large-bucket"
	objectKey := "large-file.bin"

	// Create 1MB of data
	size := 1024 * 1024
	largeContent := make([]byte, size)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	// Create bucket
	_, _ = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})

	// Put large object
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(largeContent),
	})
	if err != nil {
		t.Fatalf("PutObject large file failed: %v", err)
	}

	// Get large object
	getResp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("GetObject large file failed: %v", err)
	}
	defer getResp.Body.Close()

	body, _ := io.ReadAll(getResp.Body)
	if len(body) != size {
		t.Errorf("large file size mismatch: got %d, want %d", len(body), size)
	}
}

// Run all tests with summary
func TestMain(m *testing.M) {
	fmt.Println("Running S3 SDK Integration Tests...")
	m.Run()
}
