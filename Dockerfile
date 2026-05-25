# syntax=docker/dockerfile:1

# Compile on the CI host; cross-compile arm64 via GOARCH (avoids QEMU emulation hang).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/dsproxy ./cmd/dsproxy

FROM alpine:3.21
RUN apk add --no-cache ca-certificates su-exec \
    && adduser -D -u 65532 -h /home/nonroot nonroot
COPY --from=build /out/dsproxy /usr/local/bin/dsproxy
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh
WORKDIR /home/nonroot
EXPOSE 9999
ENV HOST=0.0.0.0
ENV PORT=9999
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
