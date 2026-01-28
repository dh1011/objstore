# Build Stage
FROM docker.io/golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
# COPY go.sum ./ # No dependencies yet, but good practice
COPY main.go ./

RUN go build -o objstore main.go

# Runtime Stage
FROM docker.io/alpine:latest

WORKDIR /app
COPY --from=builder /app/objstore .

# Create data directory
RUN mkdir -p /data

# Environment variables
ENV PORT=8080
ENV DATA_DIR=/data

EXPOSE 8080
VOLUME ["/data"]

CMD ["./objstore"]
