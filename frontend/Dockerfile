ARG VERSION=development
ARG VCS_REF=unknown

FROM oven/bun:1.2.22 AS build
ARG VERSION
WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY . .
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN bun run build && printf '%s\n' "$VERSION" > dist/VERSION

FROM nginx:1.27.5-alpine
ARG VERSION
ARG VCS_REF
LABEL org.opencontainers.image.title="Sub2API Console Frontend" \
      org.opencontainers.image.description="Sub2API Console web frontend" \
      org.opencontainers.image.source="https://github.com/MIEnchating/sub2api-console" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$VCS_REF"
RUN addgroup -g 10001 -S console && addgroup nginx console
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY docker-entrypoint.d/40-configure-trusted-proxies.sh /docker-entrypoint.d/40-configure-trusted-proxies.sh
EXPOSE 80
