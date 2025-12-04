# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build argument for version
ARG VERSION=dev

# Build the binary
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o reportportal-mcp-server ./cmd/reportportal-mcp-server

# Final stage
FROM gcr.io/distroless/base-debian12

WORKDIR /server

# Copy the binary from builder
COPY --from=builder /build/reportportal-mcp-server .

# Command to run the server
CMD ["./reportportal-mcp-server"]
