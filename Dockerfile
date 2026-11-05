FROM golang:1.27-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o tango .

FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata aria2

COPY --from=builder /build/tango /usr/local/bin/tango

RUN adduser -D -h /tango tango && \
    mkdir -p /tango/data /tango/session && \
    chown -R tango:tango /tango
USER tango

VOLUME ["/tango/data"]

# API + Web UI
EXPOSE 7246
# BitTorrent peer port
EXPOSE 6881

ENTRYPOINT ["tango", "--config", "/tango/config.yaml"]
