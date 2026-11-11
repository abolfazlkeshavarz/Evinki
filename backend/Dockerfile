# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
# builds on a development PC in a region where proxy.golang.org is blocked: set GOPROXY in .env to a mirror
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/evinkictl ./cmd/evinkictl \
 && go build -trimpath -ldflags="-s -w" -o /out/smoke ./cmd/smoke

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl && adduser -D -u 10001 evinki \
 && mkdir -p /data/uploads /data/updates && chown -R evinki /data
ENV TZ=Asia/Tehran UPLOAD_DIR=/data/uploads APP_ADDR=:8080
COPY --from=build /out/ /usr/local/bin/
USER evinki
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=5 CMD curl -fs http://127.0.0.1:8080/healthz || exit 1
CMD ["api"]
