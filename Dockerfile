# syntax=docker/dockerfile:1

FROM golang:1.23 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod/cache go mod download
COPY *.go ./
COPY templates templates
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=1 GOOS=linux go build -o /htmxchat

FROM debian:12-slim
RUN mkdir /app && chown 1000:1000 -R /app
USER 1000:1000
WORKDIR /app
COPY --from=build /htmxchat ./
EXPOSE 8080
COPY static static
CMD ["./htmxchat"]
