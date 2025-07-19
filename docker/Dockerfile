# Multi-stage build with testing

# Stage 1: Test stage
FROM golang:1.23-alpine AS test
WORKDIR /app

# Install dependencies for testing
RUN apk update && apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Run tests to ensure nothing is broken
RUN go test ./...

# Stage 2: Build stage
FROM test AS build

# Build the binary
RUN GOOS=linux GOARCH=amd64 go build

# Stage 3: Final runtime stage
FROM alpine:latest AS runtime

RUN apk update && apk add --no-cache libxml2-utils ca-certificates

# Copy the binary from build stage
COPY --from=build /app/parse-test-reports /usr/local/bin/parse-test-reports
RUN chmod +x /usr/local/bin/parse-test-reports

ENTRYPOINT ["/usr/local/bin/parse-test-reports"]