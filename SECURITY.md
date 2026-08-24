# Security

## Network boundary

MeowDeck is a LAN-only service. It must stay behind the OpenWrt nginx listener
and must not be exposed through WAN port forwarding.

Discovery targets are derived only from explicitly allowed OpenWrt logical LAN
interfaces. Private address ranges do not authorize scanning. V1 does not scan
IPv6, arbitrary ports, arbitrary URLs, or the WAN interface.

HTTP discovery does not follow redirects, loads no external assets, reads at
most 4 KiB, and renders discovered names using DOM textContent.

## Read-only surface

The application exposes GET endpoints only. It has no account system, service
CRUD, custom probes, reverse proxy, plugin execution, or user-provided target
URLs.

## Update trust

The router accepts an update only after checking:

- Ed25519 signature against an embedded key selected by key_id
- expected release channel
- a sequence newer than last_accepted_sequence
- artifact architecture, byte size, and SHA-256
- candidate self-test
- post-restart health and exact version

The final rename occurs inside /usr/bin, and the previous known-good binary is
retained until a later update succeeds.

## Reporting

Report vulnerabilities privately through GitHub Security Advisories. Do not
include router credentials, management URLs, MAC addresses, or private LAN
inventory in a public issue.
