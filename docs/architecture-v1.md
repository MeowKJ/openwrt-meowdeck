# MeowDeck Auto-NAV V1 Architecture Freeze

## Scope

MeowDeck V1 is a read-only, IPv4-only LAN navigation service for OpenWrt. It
discovers clients and a small allowlist of common web services, keeps hot probe
state in RAM, persists confirmed incidents at low frequency, and updates itself
from signed GitHub Releases.

The deployed system is one Go binary with HTML, compiled JavaScript, and CSS
embedded at build time. OpenWrt never runs Node.js, npm, TypeScript, or Go.

## Non-goals

V1 does not provide accounts, service CRUD, arbitrary URLs, arbitrary port
scanning, WAN scanning, reverse proxying, custom probes, plugins, notifications,
WebSockets, a database, Docker integration, or long-term raw heartbeat storage.

## Identity and state

- A device ID is derived from `network-domain + MAC`, never from an IP address.
- Devices move `candidate -> online -> offline`; two sightings establish a
  device and three misses mark it offline.
- Services move `candidate -> established`; two complete discovery rounds
  establish a service. Candidate services are visible but cannot create an
  incident.
- Established services require three failures to open an incident and two
  successes to recover.
- A device-offline incident suppresses incidents for services on that device.
- V1 only obtains targets from explicitly allowed OpenWrt logical LAN
  interfaces. Private address ranges alone never authorize a target.

## Discovery cadence

- Device discovery: every 60 seconds.
- New-device service discovery: immediately.
- Established-service health check: every 60 seconds.
- Complete allowlisted port rediscovery: every 10 minutes.
- New clients should appear within 60 seconds. Services already running on a
  new client should appear within 3 minutes. A service newly started on an
  existing client should appear within 12 minutes.

HTTP discovery reads at most 4 KiB, never follows redirects, never loads page
assets, and renders extracted titles with `textContent`.

## Persistence

Raw probe points, discovery caches, counters, and current state stay in RAM.
Flash stores only confirmed incident boundaries and compact observed-availability
summaries. Normal days do not cause unconditional writes. Dirty summaries are
batched at most weekly; incident changes are event driven and rate limited.

The state file is capped at 256 KiB and replaced using a temporary file in the
same directory, file `fsync`, rename, and directory `fsync`. The previous valid
state is retained. `persistence_write_count` is an in-memory diagnostic.

## Update trust and rollback

`meowdeck update` implements downloads, Ed25519 verification, SHA-256, archive
handling, anti-rollback sequence checks, self-test, installation, and rollback
using the Go standard library. The shell `meowdeck-update` command only execs
the Go subcommand.

The signed manifest includes schema, channel, version, monotonically increasing
sequence, creation time, minimum updater version, key ID, and per-architecture
artifact metadata. A manifest is rejected when its signature is invalid, its
channel differs, its sequence is not newer, its key is untrusted, or its
artifact does not match size and SHA-256.

Downloads may use `/tmp`, but the final `.meowdeck.new` file is copied and
synced inside `/usr/bin` before the atomic rename. The last-known-good binary is
kept until a later upgrade succeeds.

## Resource budgets

- Embedded frontend: at most 50 KiB gzip.
- Stripped binary: at most 8 MiB.
- Release archive: target 3 MiB, hard limit 4 MiB.
- Resident memory: at most 20 MiB on the target router.
- Persisted state: at most 256 KiB.
- Typical persistence operations: below 300 per year; hard target below 500.

