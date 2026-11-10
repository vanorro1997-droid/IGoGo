# Stage 1: Final runtime image with everything
FROM alpine:3.18
WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

# Copy the Go API binary
COPY main.go /app/main.go

# Copy build tools to compile Go
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY main.go go.mod* ./
RUN CGO_ENABLED=0 GOOS=linux go build -o api .

# Final runtime stage
FROM alpine:3.18
WORKDIR /app
RUN apk add --no-cache ca-certificates

# Copy Go binary
COPY --from=builder /app/api ./api

# Create frontend directory and copy HTML
RUN mkdir -p frontend
COPY frontend/index.html frontend/

# Expose port 8081
EXPOSE 8081

# Environment variables
ENV PORT=8081 \
    DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=postgres \
    DB_PASSWORD=postgres \
    DB_NAME=igogo

CMD ["./api"]
