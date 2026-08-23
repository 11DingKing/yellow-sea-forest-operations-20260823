# syntax=docker/dockerfile:1.7
FROM golang:1.23.12-alpine3.22 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/forest-operations ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 lightapp \
    && adduser -S -D -H -u 10001 -G lightapp lightapp \
    && mkdir -p /data \
    && chown lightapp:lightapp /data
COPY --from=build /out/forest-operations /usr/local/bin/forest-operations
USER lightapp
WORKDIR /data
ENV FOREST_ADDR=:8080 \
    FOREST_DATABASE_URL=file:/data/forest-operations.db?_pragma=foreign_keys(1)\&_pragma=busy_timeout(5000)\&_pragma=journal_mode(WAL)
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/forest-operations"]
