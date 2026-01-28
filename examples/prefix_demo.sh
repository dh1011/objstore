#!/bin/bash
set -e

# Cleanup previous run
rm -rf data
mkdir -p data

# Start server
../objstore &
PID=$!
sleep 2

echo "Storage Server (S3 Mode) started with PID $PID"
echo "----------------------------------------"

function s3cmd() {
    aws --endpoint-url http://localhost:8080 s3 "$@"
}
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

# 1. Create Bucket
s3cmd mb s3://my-bucket

# 2. Upload with Prefixes (simulating folders)
echo "doc1" > doc1.txt
echo "doc2" > doc2.txt
echo "img1" > img1.jpg
echo "img2" > img2.jpg

# "Root" level
s3cmd cp doc1.txt s3://my-bucket/doc1.txt

# "documents/" folder
s3cmd cp doc1.txt s3://my-bucket/documents/doc1.txt
s3cmd cp doc2.txt s3://my-bucket/documents/subfolder/doc2.txt

# "images/" folder
s3cmd cp img1.jpg s3://my-bucket/images/img1.jpg
s3cmd cp img2.jpg s3://my-bucket/images/2026/img2.jpg

echo "----------------------------------------"
echo "files uploaded."
echo "----------------------------------------"

# 3. List All (No prefix)
echo "3. List All (No prefix):"
s3cmd ls s3://my-bucket/ --recursive
echo "----------------------------------------"

# 4. List 'documents/' Prefix
echo "4. List 'documents/' Prefix:"
s3cmd ls s3://my-bucket/documents/ --recursive
echo "----------------------------------------"

# 5. List 'images/' Prefix
echo "5. List 'images/' Prefix:"
s3cmd ls s3://my-bucket/images/ --recursive
echo "----------------------------------------"

# Cleanup
kill $PID
rm doc1.txt doc2.txt img1.jpg img2.jpg
echo "✅ Prefix Verify Complete"
