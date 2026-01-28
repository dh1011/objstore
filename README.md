# S3-Compatible Object Storage

A lightweight, file-based S3-compatible object storage server written in Go.
It implements the essential S3 XML protocol, allowing you to use standard S3 clients (like `aws s3` CLI, MinIO client, etc.) to manage buckets and objects.

## Features

- **S3 Compatibility**: Supports `ListBuckets`, `CreateBucket`, `DeleteBucket`, `ListObjects`, `PutObject`, `GetObject`, `DeleteObject`, `HeadObject`.
- **Buckets**: Logical separation of objects using directories.
- **Flat File Storage**: Objects are stored as regular files on disk.
- **Metadata**: Stores Content-Type and custom metadata in sidecar files.

## Quick Start

### 1. Build and Run

```bash
go build -o objstore
./objstore
```
Server runs on port `8080` by default. data is stored in `./data`.

### 2. Configure AWS CLI

You can use the AWS CLI to interact with the server. You'll need to configure a dummy profile (credentials are ignored by the server but required by the CLI).

```bash
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_DEFAULT_REGION=us-east-1

# Verify strictly with endpoint-url
alias s3cmd="aws --endpoint-url http://localhost:8080 s3"
```

### 3. Usage Examples

```bash
# Create a bucket
s3cmd mb s3://my-bucket

# List buckets
s3cmd ls

# Upload a file
echo "Hello World" > hello.txt
s3cmd cp hello.txt s3://my-bucket/

# List objects
s3cmd ls s3://my-bucket/

# Download a file
s3cmd cp s3://my-bucket/hello.txt -

# Delete object
s3cmd rm s3://my-bucket/hello.txt

# Delete bucket
s3cmd rb s3://my-bucket
```

## Connecting a Web App

The server supports CORS and can be used directly from browser applications.

### Configuration
- **Endpoint**: `http://localhost:8080`
- **Force Path Style**: `true` (Required)
- **Region**: `us-east-1` (or any valid region)
- **Credentials**: Any dummy strings (e.g. `test`/`test`)

### Example (AWS SDK v3 for JS)
```javascript
import { S3Client } from "@aws-sdk/client-s3";

const s3 = new S3Client({
  region: "us-east-1",
  endpoint: "http://localhost:8080",
  credentials: {
    accessKeyId: "test",
    secretAccessKey: "test"
  },
  forcePathStyle: true // REQUIRED: Treats bucket as path segment
});
```

## Running the Demo

A `demo.sh` script is included to verify functionality:

```bash
./demo.sh
```

## Storage Layout

```
data/
├── my-bucket/
│   ├── image.png
│   └── .image.png.meta.json  # Metadata sidecar
└── backups/
```

## License

MIT
