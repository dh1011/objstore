package main

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultPort    = "8080"
	defaultDataDir = "./data"
	s3Namespace    = "http://s3.amazonaws.com/doc/2006-03-01/"
	iso8601Format  = "2006-01-02T15:04:05.000Z"
)

// --- XML Structures for S3 Response ---

type ListAllMyBucketsResult struct {
	XMLName string `xml:"ListAllMyBucketsResult"`
	Xmlns   string `xml:"xmlns,attr"`
	Owner   GenericUser
	Buckets []Bucket `xml:"Buckets>Bucket"`
}

type Bucket struct {
	Name         string
	CreationDate string
}

type GenericUser struct {
	ID          string
	DisplayName string
}

type ListBucketResult struct {
	XMLName        string `xml:"ListBucketResult"`
	Xmlns          string `xml:"xmlns,attr"`
	Name           string
	Prefix         string
	KeyCount       int
	MaxKeys        int
	IsTruncated    bool
	Contents       []Object
	CommonPrefixes []CommonPrefix
}

type Object struct {
	Key          string
	LastModified string
	ETag         string
	Size         int64
	StorageClass string
	Owner        GenericUser
}

type CommonPrefix struct {
	Prefix string
}

type ErrorResponse struct {
	XMLName   string `xml:"Error"`
	Code      string
	Message   string
	Resource  string
	RequestId string
}

// --- Storage Engine ---

type ObjectStore struct {
	dataDir string
}

func NewObjectStore(dataDir string) (*ObjectStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	return &ObjectStore{dataDir: dataDir}, nil
}

func (s *ObjectStore) bucketPath(bucket string) string {
	return filepath.Join(s.dataDir, bucket)
}

func (s *ObjectStore) objectPath(bucket, key string) string {
	return filepath.Join(s.dataDir, bucket, key)
}

func (s *ObjectStore) metaPath(bucket, key string) string {
	// Sidecar metadata file
	dir := filepath.Dir(s.objectPath(bucket, key))
	base := filepath.Base(key)
	return filepath.Join(dir, "."+base+".meta.json")
}

// Metadata structure to persist
type ObjMeta struct {
	ContentType  string
	ETag         string
	UserMetadata map[string]string
}

func (s *ObjectStore) ListBuckets() ([]Bucket, error) {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, err
	}

	var buckets []Bucket
	for _, entry := range entries {
		if entry.IsDir() {
			info, _ := entry.Info()
			buckets = append(buckets, Bucket{
				Name:         entry.Name(),
				CreationDate: info.ModTime().UTC().Format(iso8601Format),
			})
		}
	}
	return buckets, nil
}

func (s *ObjectStore) CreateBucket(name string) error {
	path := s.bucketPath(name)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return fmt.Errorf("BucketAlreadyExists")
	}
	return os.MkdirAll(path, 0755)
}

func (s *ObjectStore) DeleteBucket(name string) error {
	path := s.bucketPath(name)
	// Check if empty
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("NoSuchBucket")
		}
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("BucketNotEmpty")
	}
	return os.Remove(path)
}

func (s *ObjectStore) PutObject(bucket, key string, data io.Reader, meta ObjMeta) error {
	if _, err := os.Stat(s.bucketPath(bucket)); os.IsNotExist(err) {
		return fmt.Errorf("NoSuchBucket")
	}

	objPath := s.objectPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(objPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(objPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Calculate ETag (MD5 usually, but we use what's provided or simple calc)
	// S3 uses MD5 hex for ETag. We'll just compute SHA256 for internal use or rely on caller if we wanted.
	// For simplicity, let's just write.
	if _, err := io.Copy(f, data); err != nil {
		return err
	}

	// Save metadata
	metaBytes, _ := json.Marshal(meta)
	os.WriteFile(s.metaPath(bucket, key), metaBytes, 0644)

	return nil
}

func (s *ObjectStore) GetObject(bucket, key string) (io.ReadCloser, ObjMeta, int64, time.Time, error) {
	objPath := s.objectPath(bucket, key)
	f, err := os.Open(objPath)
	if err != nil {
		return nil, ObjMeta{}, 0, time.Time{}, fmt.Errorf("NoSuchKey")
	}

	stat, _ := f.Stat()

	// Try read metadata
	var meta ObjMeta
	metaBytes, err := os.ReadFile(s.metaPath(bucket, key))
	if err == nil {
		json.Unmarshal(metaBytes, &meta)
	}

	return f, meta, stat.Size(), stat.ModTime(), nil
}

func (s *ObjectStore) DeleteObject(bucket, key string) error {
	objPath := s.objectPath(bucket, key)
	os.Remove(objPath)
	os.Remove(s.metaPath(bucket, key))
	// Remove empty parent dirs? optional.
	return nil
}

// ListObjects returns simple list - no complex delimiter support for now
func (s *ObjectStore) ListObjects(bucket, prefix string) ([]Object, error) {
	bucketDir := s.bucketPath(bucket)
	if _, err := os.Stat(bucketDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("NoSuchBucket")
	}

	var objects []Object
	err := filepath.Walk(bucketDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Skip metadata files
		if strings.HasPrefix(filepath.Base(path), ".") && strings.HasSuffix(path, ".meta.json") {
			return nil
		}

		rel, _ := filepath.Rel(bucketDir, path)
		if prefix != "" && !strings.HasPrefix(rel, prefix) {
			return nil
		}

		objects = append(objects, Object{
			Key:          rel,
			LastModified: info.ModTime().UTC().Format(iso8601Format),
			Size:         info.Size(),
			ETag:         `"placeholder-etag"`, // clients expect quotes
			StorageClass: "STANDARD",
			Owner:        GenericUser{ID: "admin", DisplayName: "admin"},
		})
		return nil
	})
	return objects, err
}

// --- Handler ---

func writeError(w http.ResponseWriter, code, msg, resource string, status int) {
	w.WriteHeader(status)
	xml.NewEncoder(w).Encode(ErrorResponse{
		Code:     code,
		Message:  msg,
		Resource: resource,
	})
}

func (s *ObjectStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// S3 buckets can be via domain (bucket.host) or path (host/bucket).
	// We assume path style: /bucket/key...
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)

	// Root: List Buckets
	if r.URL.Path == "/" || r.URL.Path == "" {
		s.handleListBuckets(w, r)
		return
	}

	bucket := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}

	if key == "" {
		// Bucket Ops
		switch r.Method {
		case http.MethodGet:
			s.handleListObjects(w, r, bucket)
		case http.MethodPut:
			s.handleCreateBucket(w, r, bucket)
		case http.MethodDelete:
			s.handleDeleteBucket(w, r, bucket)
		default:
			writeError(w, "MethodNotAllowed", "Method not allowed", bucket, http.StatusMethodNotAllowed)
		}
	} else {
		// Object Ops
		switch r.Method {
		case http.MethodGet:
			s.handleGetObject(w, r, bucket, key)
		case http.MethodPut:
			s.handlePutObject(w, r, bucket, key)
		case http.MethodDelete:
			s.handleDeleteObject(w, r, bucket, key)
		case http.MethodHead:
			s.handleHeadObject(w, r, bucket, key)
		default:
			writeError(w, "MethodNotAllowed", "Method not allowed", bucket+"/"+key, http.StatusMethodNotAllowed)
		}
	}
}

func (s *ObjectStore) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := s.ListBuckets()
	if err != nil {
		writeError(w, "InternalError", err.Error(), "", 500)
		return
	}

	resp := ListAllMyBucketsResult{
		Xmlns:   s3Namespace,
		Owner:   GenericUser{ID: "admin", DisplayName: "admin"},
		Buckets: buckets,
	}
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(resp)
}

func (s *ObjectStore) handleCreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := s.CreateBucket(bucket); err != nil {
		if err.Error() == "BucketAlreadyExists" {
			writeError(w, "BucketAlreadyExists", "Bucket already exists", bucket, 409)
		} else {
			writeError(w, "InternalError", err.Error(), bucket, 500)
		}
		return
	}
	w.WriteHeader(200) // S3 CreateBucket returns 200 OK
}

func (s *ObjectStore) handleDeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := s.DeleteBucket(bucket); err != nil {
		if err.Error() == "NoSuchBucket" {
			writeError(w, "NoSuchBucket", "The specified bucket does not exist", bucket, 404)
		} else if err.Error() == "BucketNotEmpty" {
			writeError(w, "BucketNotEmpty", "The bucket is not empty", bucket, 409)
		} else {
			writeError(w, "InternalError", err.Error(), bucket, 500)
		}
		return
	}
	w.WriteHeader(204)
}

func (s *ObjectStore) handleListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	prefix := r.URL.Query().Get("prefix")
	objects, err := s.ListObjects(bucket, prefix)
	if err != nil {
		if err.Error() == "NoSuchBucket" {
			writeError(w, "NoSuchBucket", "The specified bucket does not exist", bucket, 404)
		} else {
			writeError(w, "InternalError", err.Error(), bucket, 500)
		}
		return
	}

	resp := ListBucketResult{
		Xmlns:       s3Namespace,
		Name:        bucket,
		Prefix:      prefix,
		KeyCount:    len(objects),
		MaxKeys:     1000,
		IsTruncated: false,
		Contents:    objects,
	}
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(resp)
}

func (s *ObjectStore) handlePutObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	meta := ObjMeta{
		ContentType: r.Header.Get("Content-Type"),
		// In real usage, calculate MD5 of body for ETag
		ETag: fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(time.Now().String()))),
	}

	if err := s.PutObject(bucket, key, r.Body, meta); err != nil {
		if err.Error() == "NoSuchBucket" {
			writeError(w, "NoSuchBucket", "Bucket not found", bucket, 404)
		} else {
			writeError(w, "InternalError", err.Error(), key, 500)
		}
		return
	}
	w.Header().Set("ETag", meta.ETag)
	w.WriteHeader(200)
}

func (s *ObjectStore) handleGetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	f, meta, size, modTime, err := s.GetObject(bucket, key)
	if err != nil {
		writeError(w, "NoSuchKey", "The specified key does not exist", key, 404)
		return
	}
	defer f.Close()

	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	if meta.ETag != "" {
		w.Header().Set("ETag", meta.ETag)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))

	io.Copy(w, f)
}

func (s *ObjectStore) handleDeleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	s.DeleteObject(bucket, key)
	w.WriteHeader(204)
}

func (s *ObjectStore) handleHeadObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	f, meta, size, modTime, err := s.GetObject(bucket, key)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	f.Close()

	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
	w.WriteHeader(200)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	store, err := NewObjectStore(dataDir)
	if err != nil {
		log.Fatal(err)
	}

	addr := ":" + port
	log.Printf("🔥 S3-Compatible Server starting on %s", addr)
	log.Printf("📂 Data dir: %s", dataDir)

	// Add CORS middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for all responses
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*, Authorization, Content-Type, Content-Length, x-amz-date, x-amz-content-sha256, x-amz-user-agent")
		w.Header().Set("Access-Control-Expose-Headers", "ETag, x-amz-request-id")

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		store.ServeHTTP(w, r)
	})

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
