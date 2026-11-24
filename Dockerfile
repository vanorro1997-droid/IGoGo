# Pin multi-platform indexes; registry overrides are documented in docs/docker.md.
ARG GO_IMAGE=public.ecr.aws/docker/library/golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc
ARG ALPINE_IMAGE=public.ecr.aws/docker/library/alpine:3.23.8@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2

# Stage 1: build the Go API
FROM ${GO_IMAGE} AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o api .

# Final runtime stage
FROM ${ALPINE_IMAGE}
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata

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
