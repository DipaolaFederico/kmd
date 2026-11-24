# Orbit handbook

Orbit is a small, made-up service used to show kmd's viewer. Its handbook
is a folder of Markdown files that link to each other, like a wiki.

- [Setting up](setup.md) — install Orbit and start it
- [Architecture](architecture.md) — how the pieces talk to each other
- [Operations](#operations) — day-to-day tasks, further down this page

## Quick start

```sh
orbit init my-station
orbit run --port 8080
```

Open <http://localhost:8080> and sign in with the token that `orbit init`
printed.

## Concepts

| Term     | Meaning                                    |
|----------|--------------------------------------------|
| Station  | one running Orbit instance                 |
| Payload  | a unit of work a station accepts           |
| Ground   | the database every station writes to       |

> [!TIP]
> Press `t` in kmd for the table of contents and `[` `]` to jump
> between sections.

## Operations

### Backups

`orbit backup` writes a consistent snapshot of the ground database.
Keep at least seven daily snapshots.

### Upgrades

1. Read the release notes.
2. Back up the ground database.
3. Replace the binary and restart the station.

### Monitoring

Every station serves its metrics at `/metrics`. Alert when the payload
queue stays above 1000 for more than ten minutes.

## Troubleshooting

**The station does not start.** Check that the port is free and that the
ground database is reachable.

**Payloads stay queued.** A worker crashed: `orbit workers --restart`.
