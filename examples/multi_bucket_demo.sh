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

# 1. Create Multiple Buckets
echo "1. Creating buckets..."
s3cmd mb s3://bucket-alpha
s3cmd mb s3://bucket-beta
s3cmd mb s3://bucket-gamma
echo "----------------------------------------"

# 2. List Buckets
echo "2. Listing all buckets..."
s3cmd ls
echo "----------------------------------------"

# 3. Upload Objects to Different Buckets
echo "3. Uploading objects..."
echo "file in alpha" > alpha.txt
echo "file in beta" > beta.txt
s3cmd cp alpha.txt s3://bucket-alpha/
s3cmd cp beta.txt s3://bucket-beta/
echo "----------------------------------------"

# 4. List Objects per Bucket
echo "4. Listing objects in bucket-alpha..."
s3cmd ls s3://bucket-alpha/
echo -e "\n4. Listing objects in bucket-beta..."
s3cmd ls s3://bucket-beta/
echo "----------------------------------------"

# Cleanup
kill $PID
rm alpha.txt beta.txt
echo "✅ Multiple Buckets Verify Complete"
