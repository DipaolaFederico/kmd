# Setting up

Back to the [handbook](README.md).

## Requirements

- Linux, macOS or FreeBSD
- 512 MiB of memory per station
- a PostgreSQL 16 server for the ground database

## Install

```sh
curl -fsSLO https://example.com/orbit/orbit-linux-amd64.tar.gz
tar -xzf orbit-linux-amd64.tar.gz
sudo install orbit /usr/local/bin/
```

## Configure

Orbit reads `~/.config/orbit/station.toml`:

```toml
[station]
name = "my-station"
port = 8080

[ground]
url = "postgres://orbit@localhost/orbit"
```

> [!WARNING]
> Never commit the ground database password; use `ORBIT_GROUND_PASSWORD`.

## Start

`orbit run` starts the station in the foreground. See
[Architecture](architecture.md) for what happens next.
