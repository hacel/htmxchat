# syntax=docker/dockerfile:1

FROM golang:1.23 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY templates templates
RUN CGO_ENABLED=1 GOOS=linux go build -o /htmxchat

FROM gcr.io/distroless/base-debian12 AS build-release-stage
USER nonroot:nonroot
WORKDIR /app
COPY --from=build /htmxchat ./
EXPOSE 8080
COPY static static
CMD ["./htmxchat"]
