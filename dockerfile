# Step 1: Build the Go app
# Use the official Golang image to build the application
FROM golang:1.20-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum (if they exist) to cache dependencies
COPY go.mod go.sum ./

# Download and cache dependencies (this step is faster during rebuilds)
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the Go app
RUN go build -o iot-app .

# Step 2: Run the Go app
# Use a smaller base image to run the application
FROM alpine:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the compiled Go binary from the builder stage
COPY --from=builder /app/iot-app .

# Expose a port (if needed, e.g., for a web server)
EXPOSE 9090

# Run the app
CMD ["./iot-app"]
