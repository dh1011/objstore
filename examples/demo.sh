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

# Configure aws alias for local usage
# We use a dummy profile to avoid messing with user's real config, 
# and pass explicit credentials to avoid looking them up.
function s3cmd() {
    aws --endpoint-url http://localhost:8080 s3 "$@"
}
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

# 1. List Buckets (Should be empty)
echo "1. List Buckets (Empty)..."
s3cmd ls
echo "----------------------------------------"

# 2. Create Bucket
echo "2. Create Bucket 'mybucket'..."
s3cmd mb s3://mybucket
echo "----------------------------------------"

# 3. List Buckets (Should show mybucket)
echo "3. List Buckets..."
s3cmd ls
echo "----------------------------------------"

# 4. Upload Object
echo "4. Uploading 'hello.txt'..."
echo "Hello S3" > hello.txt
s3cmd cp hello.txt s3://mybucket/hello.txt
echo "----------------------------------------"

# 5. List Objects
echo "5. List Objects in 'mybucket'..."
s3cmd ls s3://mybucket/
echo "----------------------------------------"

# 6. Download Object
echo "6. Downloading 'downloaded.txt'..."
s3cmd cp s3://mybucket/hello.txt downloaded.txt
cat downloaded.txt
echo -e "\n----------------------------------------"

# 7. Delete Object
echo "7. Deleting 'hello.txt'..."
s3cmd rm s3://mybucket/hello.txt
echo "----------------------------------------"

# 8. Delete Bucket
echo "8. Deleting 'mybucket'..."
s3cmd rb s3://mybucket
echo "----------------------------------------"

# Cleanup
kill $PID
rm hello.txt downloaded.txt
echo "✅ S3 Compatibility Demo Complete"
