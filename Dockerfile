# Build the manager binary
FROM golang:1.21 as builder

WORKDIR /workspace

# Copy the Go Modules manifests and other source files
COPY go.mod go.sum ./
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Download dependencies and verify them
RUN go mod tidy && go mod verify

# Build the Go binary in a way that's suitable for running within a minimal base image
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /

# Copy the binary from the builder stage
COPY --from=builder /workspace/manager .

# Non-root user
USER nonroot:nonroot

# Set the binary as the entrypoint of the container
ENTRYPOINT ["/manager"]
