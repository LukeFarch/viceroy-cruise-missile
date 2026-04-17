## Summary
<!-- What does this change and why? Keep it tight. -->

## Checklist
- [ ] `make fmt-check` passes
- [ ] `go build ./cmd/...` succeeds
- [ ] `docker compose config --quiet` passes for both swarm files
- [ ] No secrets, tailnet IPs, or hardcoded passwords in the diff
- [ ] Configs regenerated if daemon code changed (`make configs-all`)
- [ ] `docs/FIDELITY.md` updated if this changes behavior relative to the real competition
- [ ] New Go files carry the `SPDX-License-Identifier: GPL-3.0-or-later` header

## Fidelity note
<!-- Skip if N/A. Otherwise: does this move behavior toward or away from the real event, and how should the fidelity doc change? -->

## Attack-surface note
<!-- Skip if N/A. If this changes any item in WHITEPAPER.md §7 (scenario attack surface), justify why. -->
