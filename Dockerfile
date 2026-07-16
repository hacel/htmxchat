# syntax=docker/dockerfile:1.7

FROM node:24-bookworm-slim AS assets
WORKDIR /src
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --ignore-scripts
COPY templates/input.css templates/templates.templ ./templates/
COPY static/app.js ./static/app.js
COPY third_party ./third_party
RUN npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY *.go ./
COPY templates ./templates
COPY --from=assets /src/static ./static
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/htmxchat .

FROM debian:bookworm-slim
RUN groupadd --gid 65532 htmxchat \
    && useradd --uid 65532 --gid 65532 --no-create-home --shell /usr/sbin/nologin htmxchat \
    && install -d -o htmxchat -g htmxchat /var/lib/htmxchat
COPY --from=build --chown=htmxchat:htmxchat --chmod=0555 /out/htmxchat /usr/local/bin/htmxchat
USER htmxchat:htmxchat
WORKDIR /var/lib/htmxchat
ENV HTMXCHAT_LISTEN_ADDRESS=:8080 \
    HTMXCHAT_DATABASE_PATH=/var/lib/htmxchat/sqlite.db
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/htmxchat"]
