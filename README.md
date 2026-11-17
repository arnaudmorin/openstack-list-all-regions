# ola — OpenStack List All

`ola` lists the servers of your OpenStack project across **every region** in
the service catalog, queried in parallel, and prints them as a table.

```
╭───────────┬──────────────────────────────────────┬───────┬───────────────────────┬────────────────────╮
│ REGION    │ UUID                                 │ NAME  │ IPS                   │ STATUS             │
├───────────┼──────────────────────────────────────┼───────┼───────────────────────┼────────────────────┤
│ GRA3      │ 5d78123d-f896-44b2-addd-794e08cfd37b │ web-1 │ Ext-Net: 203.0.113.10 │       ACTIVE       │
│ SBG7      │ ddca1f38-1d24-4d61-af13-ef0d436e214b │ db-1  │ Ext-Net: 203.0.113.20 │        ERROR       │
├───────────┼──────────────────────────────────────┼───────┼───────────────────────┼────────────────────┤
│ 2 servers │                                      │       │                       │ 1 ACTIVE · 1 ERROR │
╰───────────┴──────────────────────────────────────┴───────┴───────────────────────┴────────────────────╯
```

## Features

- Discovers regions from the public `compute` endpoints of the service catalog.
- Queries all regions concurrently (up to 40 at a time).
- Sorted by region, then server name.
- In a terminal: coloured statuses (ACTIVE green, ERROR red, …), zebra-striped
  rows, one IP per line, and a footer with the server count per status.
- When piped: no colours, IPs comma-separated, so it stays `grep`-friendly.
- A failing region prints a warning on stderr and is skipped.

## Requirements

- Go 1.26+ to build (older toolchains download it automatically).
- OpenStack credentials using Keystone v3.

## Build & install

```sh
make build      # builds ./ola
make install    # installs ola into $(go env GOBIN) or $(go env GOPATH)/bin
make uninstall  # removes the installed binary
make clean      # removes ./ola
```

Install somewhere else with `make install GOBIN=/usr/local/bin`.

## Configuration

`ola` authenticates like the `openstack` CLI:

- `OS_CLOUD` pointing to an entry in `clouds.yaml`, or
- the usual `OS_*` environment variables (e.g. from a sourced `openrc.sh`).

## Usage

```sh
ola            # list servers with their IPv4 addresses
ola --ipv6     # also show IPv6 addresses
ola --debug    # show discovered regions, per-region progress and HTTP traffic on stderr
```

Note: `--debug` logs API calls made after authentication; the initial token
request is not logged.
