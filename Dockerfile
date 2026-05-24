FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dsproxy ./cmd/dsproxy

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dsproxy /usr/local/bin/dsproxy
EXPOSE 9999
USER nonroot:nonroot
ENV HOST=0.0.0.0
ENV PORT=9999
ENTRYPOINT ["/usr/local/bin/dsproxy"]
