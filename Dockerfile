# syntax=docker/dockerfile:1

# Compile on the build host; cross-compile via TARGETARCH.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/dsproxy \
    ./cmd/dsproxy


FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 65532 -h /home/nonroot nonroot \
    && mkdir -p /home/nonroot/.dsproxy \
    && chown -R nonroot:nonroot /home/nonroot

COPY --from=build --chown=nonroot:nonroot \
    /out/dsproxy \
    /usr/local/bin/dsproxy

USER nonroot:nonroot

WORKDIR /home/nonroot

EXPOSE 9999

ENV HOST=0.0.0.0 \
    PORT=9999

ENTRYPOINT ["/usr/local/bin/dsproxy"]