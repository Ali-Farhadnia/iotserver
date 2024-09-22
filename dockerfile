# Step 1: Build the Go app
# Use the appropriate Golang version and enable CGO for SQLite
FROM golang:1.21-alpine AS builder

# Install necessary dependencies for CGO (for SQLite)
RUN apk add --no-cache gcc g++ musl-dev

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to cache dependencies first
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the application code
COPY . .

# Enable CGO (required for go-sqlite3) and build the Go app
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o iotserver .

# Step 2: Run the Go app
# Use a minimal base image to run the application
FROM alpine:latest

# Install SQLite libraries required by the Go app
RUN apk add --no-cache sqlite-libs

# Set the working directory inside the container
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/iotserver .

# Expose any port your application might use (e.g., for HTTP API)
EXPOSE 9090

# Command to run the app
CMD ["./iotserver"]
