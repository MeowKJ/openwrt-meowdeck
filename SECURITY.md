# Security

MeowDeck is intended for trusted local networks. Do not expose its listener or
the router administration links directly to the public Internet.

The service-edit API is deliberately unauthenticated for a simple LAN-only
installation. `X-MeowDeck-Edit` is a cross-site request guard, not an access
token. Put MeowDeck behind an authenticated reverse proxy before using it on an
untrusted network.

Please report vulnerabilities privately through GitHub Security Advisories.
Do not include router passwords, tokens, private IP inventories, or full
configuration files in public issues.
