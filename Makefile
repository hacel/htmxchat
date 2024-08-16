all:
	templ generate
	npx tailwindcss -o ./static/main.css --minify

# run templ generation in watch mode to detect all .templ files.
live/templ:
	templ generate --watch

# run air to detect any go file changes to re-build and re-run the server.
live/server:
	air --build.exclude_dir "node_modules"

# run tailwindcss to generate the styles.css bundle in watch mode.
live/tailwind:
	npx tailwindcss -o ./static/main.css --minify --watch

# start all 3 watch processes in parallel.
live:
	make -j3 live/tailwind live/templ live/server
