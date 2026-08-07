#!/usr/bin/env python3
"""benchmarks/record.py — append one results row to the v3 results CSV.

Each row snapshots the exact startup.sh args plus the load metrics parsed from
the driver's `key=value` output, so runs are reproducible from the CSV alone.

Usage: record.py --csv FILE --tag T --orgs N ... --load-out "key=value lines"
"""
from __future__ import annotations

import argparse
import csv
import os
from datetime import datetime, timezone


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--csv", required=True)
    ap.add_argument("--tag", default="baseline")
    ap.add_argument("--orgs", default="3")
    ap.add_argument("--peers-per-org", default="1")
    ap.add_argument("--orderers", default="3")
    ap.add_argument("--server", default="baseline")
    ap.add_argument("--batch-timeout", default="2s")
    ap.add_argument("--max-message", default="10")
    ap.add_argument("--state-db", default="couchdb")
    ap.add_argument("--policy", default="2of3")
    ap.add_argument("--crypto-style", default="ecdsa")
    ap.add_argument("--concurrency", default="24")
    ap.add_argument("--rate", default="25")
    ap.add_argument("--rw-mix", default="100")
    ap.add_argument("--fault", default="none")
    ap.add_argument("--driver", default="http")
    ap.add_argument("--load-out", default="")
    args = ap.parse_args()

    fields = {
        "timestamp": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "tag": args.tag,
        "orgs": args.orgs,
        "peers_per_org": args.peers_per_org,
        "orderers": args.orderers,
        "server": args.server,
        "batch_timeout": args.batch_timeout,
        "max_message": args.max_message,
        "state_db": args.state_db,
        "policy": args.policy,
        "crypto_style": args.crypto_style,
        "concurrency": args.concurrency,
        "rate": args.rate,
        "rw_mix": args.rw_mix,
        "fault": args.fault,
        "driver": args.driver,
        "total": "",
        "elapsed_s": "",
        "achieved_tps": "",
        "p50_ms": "",
        "p95_ms": "",
        "p99_ms": "",
        "failures": "",
        "failure_rate": "",
    }
    for line in args.load_out.splitlines():
        if "=" in line:
            k, v = line.split("=", 1)
            k = k.strip()
            if k in fields:
                fields[k] = v.strip()

    header = list(fields.keys())
    write_header = not os.path.exists(args.csv) or os.path.getsize(args.csv) == 0
    with open(args.csv, "a", newline="") as f:
        w = csv.DictWriter(f, fieldnames=header)
        if write_header:
            w.writeheader()
        w.writerow(fields)
    print(f"recorded row -> {args.csv} (tag={args.tag})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
