# User Request

It seems like Caddy is enabled with the HTTP/3 protocol; this likely causes some
issues with iPhone Safari. Can we only advertise HTTP/1.x and HTTP/2?

## Context

- Decloud provisions/configures Caddy as the reverse proxy / TLS terminator.
- HTTP/3 (QUIC, UDP/443) appears to be advertised, and the suspicion is that this
  is causing connectivity issues on iPhone Safari.
- Goal: restrict the advertised protocols to HTTP/1.1 and HTTP/2 only (disable HTTP/3).
