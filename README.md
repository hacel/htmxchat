# htmxchat

A small public chat service built with Go, Echo, WebSockets, templ, htmx, and Tailwind CSS. The server embeds its browser assets, stores recent chat history in SQLite, and runs without third-party browser requests.

## Requirements

- Go 1.25 or newer
- Node.js 24 or newer
- A C compiler for `go-sqlite3`

## Development

Install the pinned dependencies and generate the frontend assets:

```sh
make deps
make generate
```

Start the service:

```sh
go run .
```

The default address is `:8080` and the default database is `var/sqlite.db`. The database directory is created automatically.

For live reload of Go, templ, and Tailwind changes, run:

```sh
make live
```

Before committing, run the full test, formatting, audit, and generated-file check:

```sh
make check
```

## Configuration

| Environment variable      | Default         | Purpose              |
| ------------------------- | --------------- | -------------------- |
| `HTMXCHAT_LISTEN_ADDRESS` | `:8080`         | HTTP listen address  |
| `HTMXCHAT_DATABASE_PATH`  | `var/sqlite.db` | SQLite database path |

The service accepts `X-Real-IP` only from a loopback reverse proxy. When deploying behind a proxy, replace that header rather than forwarding a client-supplied value.

## Container

Build and run the non-root image with a persistent database volume:

```sh
docker build -t htmxchat .
docker run --rm -p 8080:8080 -v htmxchat-data:/var/lib/htmxchat htmxchat
```

## Generated files

`templates/templates_templ.go` is generated from `templates/templates.templ`. Files under `static/` are generated or copied by `npm run build`, except for `static/app.js`. The generated files remain committed so packaging does not require Node.js.
