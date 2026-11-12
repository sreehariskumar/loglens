# LogLens

A lightweight, self-hosted browser log explorer. LogLens scans files on demand and does not require a database or external service.

## Current MVP

- Browser UI for selecting files beneath a configured log directory
- Plain-text and JSON/JSONL line viewing
- Case-insensitive keyword search and regular-expression search
- Severity filtering (TRACE, DEBUG, INFO, WARN, ERROR, FATAL)
- Time range filtering for common timestamp formats
- Local-first server defaults to `127.0.0.1`

## Requirements

- Go 1.22+

## Run

```bash
git clone https://github.com/sreehariskumar/loglens.git
cd loglens
go run . --root ./sample-logs
```

Open http://127.0.0.1:8080.

Put `.log`, `.txt`, `.json`, or `.jsonl` files in `sample-logs` (or point `--root` at an existing directory).

Build a single executable:

```bash
go build -o loglens .
./loglens --root ./logs --listen 127.0.0.1:8080
```

## Security

- The server binds to loopback by default. Do not expose it to an untrusted network without adding authentication and TLS.
- The configured `--root` is the only directory exposed through the file listing and search API.
- LogLens reads files; it does not execute their contents or upload them elsewhere.
- Treat logs as sensitive data. Avoid running with a broad root such as `/` or your home directory.

## API

`GET /api/files` — list supported files under the configured root.

`GET /api/search?file=app.log&q=timeout&level=ERROR&regex=false&from=2026-11-12%2010:00:00&to=2026-11-12%2011:00:00&limit=500`

The search endpoint scans line by line and returns at most 5,000 matching entries. The current MVP scans from the beginning for each request; it does not yet maintain an index or implement live tailing.

## Roadmap

- [ ] File upload / drag and drop
- [ ] Live tail and pause/resume
- [ ] Virtualized rendering
- [ ] Multi-file search
- [ ] Better timestamp format configuration and timezone controls
- [ ] Rust search engine after profiling
- [ ] Optional Python parser extensions
- [ ] Benchmarks and automated tests

## License

MIT. See [LICENSE](LICENSE).
