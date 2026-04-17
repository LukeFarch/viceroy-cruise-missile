# Security Policy

## This range is intentionally vulnerable

Viceroy is a training environment that simulates a distributed system with
known weaknesses. The weaknesses are the point — players learn to find and
exploit them. Do not treat this repo as hardened software.

## Operator responsibilities

Before bringing a range online:

1. **Do not expose the range to the public internet.** Remote team access
   is intended to go through the WireGuard tunnel described in
   [WireGuard Setup wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/WireGuard-Setup), or the optional Tailscale overlay in
   [Tailscale Setup wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Tailscale-Setup). Never bind the attack-station SSH port or the
   scoreboard to `0.0.0.0` on a public host.
2. **Regenerate credentials for every deployment.** Run `make configs` (or
   `make configs-all`) before `docker compose up` so each deployment has
   fresh UUIDs and Ed25519 keypairs. The repo intentionally ships with no
   pre-generated private keys.
3. **Change the default password.** `scripts/entrypoint-attack.sh` creates
   the documented team users (`dash`, `laura`, `phoenix`) with a default
   password for competition-style parity. Override `TEAM_PASS` via the
   container environment or replace the users before granting outside
   access.
4. **Isolate the Docker host.** The host running the range should not be
   dual-homed onto a network carrying non-exercise traffic.

## What constitutes a real security issue

The following are bugs we want to fix — please report them:

- A weakness in the range *infrastructure* that is not part of the
  intended training content. Examples: the scoreboard leaking real host
  data, the WireGuard generator emitting weak keys, the Docker network
  unintentionally bridging to the host LAN.
- Any path where a player session inside the range can escape the
  containers and reach the operator's host beyond what the documented
  Docker socket mount intentionally allows.
- Secrets checked into the repository that should not be there.

The following are **not** bugs — they are features:

- The Mantis daemons accept unsigned messages when `verify_signatures` is
  set to `false`. That is a scenario dial.
- The controller election protocol can be tricked by a malicious peer.
  That is the Boomer kill-chain.
- The attack-station container has broad tooling. That is the point of an
  attack station.

## Reporting

Open a GitHub issue and tag it `security`. If the issue exposes live
credentials, email the maintainers listed in the repository before opening
the issue publicly and we will coordinate a fix.

## No warranty

This project is provided under the GNU GPL v3. As stated in that license,
there is no warranty. Use at your own risk.
