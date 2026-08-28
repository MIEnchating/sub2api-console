ARG VERSION=development
ARG VCS_REF=unknown

FROM golang:1.26.6-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/sub2api-console ./cmd/server

FROM alpine:3.22
ARG VERSION
ARG VCS_REF
LABEL org.opencontainers.image.title="Sub2API Console API" \
      org.opencontainers.image.description="Sub2API Console backend API" \
      org.opencontainers.image.source="https://github.com/MIEnchating/sub2api-console" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$VCS_REF"
ENV SUB2API_CONSOLE_VERSION="$VERSION"
RUN apk add --no-cache ca-certificates su-exec tzdata \
    && addgroup -g 10001 -S console \
    && adduser -u 10001 -S -G console console
WORKDIR /app
COPY --from=builder /out/sub2api-console /usr/local/bin/sub2api-console
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN printf '%s\n' "$VERSION" > /app/VERSION
RUN mkdir -p /app/data && chown -R console:console /app
EXPOSE 8080
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["sub2api-console"]
