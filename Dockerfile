# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-X main.appVersion=${VERSION}" -o chem-resolver .

# Run stage
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates openbabel \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --no-create-home --uid 1000 chemres
WORKDIR /app
COPY --from=builder --chown=chemres:chemres /app/chem-resolver .
COPY --from=builder --chown=chemres:chemres /app/templates ./templates
COPY --from=builder --chown=chemres:chemres /app/static ./static
USER chemres
EXPOSE 8080
CMD ["./chem-resolver"]
