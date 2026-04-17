# Disclaimer

**Viceroy is an unaffiliated, community-built practice range.** It is not
produced, sponsored, endorsed, or reviewed by the VICEROY Virtual Institute,
the Department of Defense, the College of William & Mary, Cyber Outcomes
Group LLC, or any other organization associated with the official "Gilded
Guardian" competition series.

## What this project is

Viceroy replicates the public, observable shape of the Mantis swarm system
that competitors encountered during the 2025–2026 Gilded Guardian Challenge
Series. It was built by a team member who participated in the live event and
wanted to keep practicing the Boomer kill-chain afterward. The architecture,
node roles, and attack surface are reconstructed from publicly available
program descriptions, the team's own competition notes, and the team's
post-event writeup.

## What this project is not

- Not an authoritative source on the real competition.
- Not a vehicle for reproducing copyrighted materials belonging to the
  competition sponsors. Reference materials under `docs/comp-materials/`
  are retained under fair-use for educational commentary; see `NOTICE` for
  the takedown policy.
- Not representative of the exact scale, timing, or scoring of the live
  event. See [Fidelity wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/Fidelity) for a point-by-point comparison.
- Not a shortcut for VICEROY scholars attempting to game the official
  competition. Teams are still expected to meet the program's own rules
  and requirements.

## Intentionally vulnerable by design

This range ships with known weaknesses on purpose — that is the training
value. Do not run it on a shared network, a production VLAN, or anything
reachable from the public internet without the access controls documented
in [WireGuard Setup wiki](https://github.com/LukeFarch/viceroy-cruise-missile/wiki/WireGuard-Setup). See `SECURITY.md` for the full threat model.

## Trademark note

"VICEROY", "Gilded Guardian", "Mantis", "RRII", and "ELCOA" are used here
descriptively to identify the competition this practice range was built to
train against. All marks remain the property of their respective owners.
