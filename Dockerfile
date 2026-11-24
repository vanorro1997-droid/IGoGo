# Stage 1: build the Go API
FROM golang:1.26.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o api .

# Final runtime stage
FROM alpine:3.18
WORKDIR /app
RUN apk add --no-cache ca-certificates

# Copy Go binary
COPY --from=builder /app/api ./api

# Copy frontend and schema (schema is also mounted by docker-compose)
RUN mkdir -p frontend db
COPY frontend/ frontend/
COPY db/schema.sql db/

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
