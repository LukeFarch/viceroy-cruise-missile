# Contributing to Viceroy

Thanks for wanting to help. A few things to know before you open a PR.

---

## Ground rules

1. **No real secrets in PRs.** `.gitignore` covers the common cases
   (`configs/**/key.pem`, `wireguard/peers/`, `.env`). Eyeball your
   diff anyway. Default deploy credentials used by the attack-station
   entrypoint are fair game to mention; anything tied to a specific
   tailnet, WireGuard peer, or real operator is not.
2. **Regenerate configs** before submitting changes to daemon code:

    ```bash
    make configs-all
    docker compose -f docker-compose.yml -f docker-compose.swarm2.yml \
        up -d --build
    ```

3. **Keep [Fidelity wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Fidelity) honest.** If your change moves behavior
   toward or away from the real Gilded Guardian event, update the
   relevant row and the PR description.
4. **Respect the DISCLAIMER.** This repo is unaffiliated with VICEROY,
   the DoD, and Cyber Outcomes Group LLC. Do not add claims or
   branding suggesting otherwise.

---

## Local setup

```bash
# Toolchain
go version         # >= 1.23
docker --version   # >= 24
docker compose version   # v2+

# One-time
cp .env.example .env
make configs
docker compose up -d
```

Stop with `make down`. Full wipe + rebuild: `make reset-full`.

---

## What a good PR looks like

- One logical change per PR. Refactors separate from behavior changes.
- Go files formatted with `gofmt`:

    ```bash
    make fmt           # in-place format
    make fmt-check     # CI-equivalent check
    ```

- `go vet ./...` (`make lint`) is advisory. Two pre-existing copylock
  warnings in `cmd/scoreboard` and `cmd/scenario` are tracked; don't
  add new ones.
- Docker Compose validates:

    ```bash
    docker compose config --quiet
    docker compose -f docker-compose.yml -f docker-compose.swarm2.yml \
        config --quiet
    ```

- If you touched a VPN flow, dry-run the generator:

    ```bash
    ./scripts/wg-gen.sh --peers 2 --endpoint 127.0.0.1:51820
    ```

  Then delete the generated peer configs — they are gitignored but do
  not leave them on disk for others.

---

## Tests

There is no Go test suite yet. If you add one, put it next to the
package, keep it hermetic (no Docker required), and wire it into
`.github/workflows/ci.yml`. Integration tests that require a running
swarm should be opt-in via a build tag, not gated by default CI.

---

## Reviewing PRs

Maintainers check for:

- Secrets in diff.
- Fidelity document consistency.
- Whether the change needs a follow-up to `README.md` or [Whitepaper wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Whitepaper).
- Whether the change alters the attack surface (items in
  [Whitepaper wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Whitepaper) §7) — those changes need a justification in the PR
  description.

---

## Reporting issues

- **Range-infrastructure bugs** (scoreboard leaks, generator weakness,
  docker networking escapes) — open an issue with the `security` tag.
  See `SECURITY.md` for the threat model.
- **Scenario-content bugs** (nodes crash for no reason, scenario
  timing is off, scoreboard shows wrong state) — open a regular issue.
- **Requests for real-competition material** — do not open an issue.
  Viceroy does not distribute non-public competition content.

---

## Licensing of contributions

By submitting a PR you agree that your contribution will be licensed
under GPL-3.0-or-later, matching the rest of the project
(`LICENSE`). Include a `SPDX-License-Identifier: GPL-3.0-or-later`
header at the top of any new Go file.
