"""Off-chain report storage + API-key -> org mapping (v2 §3.4, §4).

The full Report objects (which include raw text) never touch the ledger — they
live off-chain. The ledger stores only a `content_hash` plus an `off_chain_uri`
pointing at the full report.

Storage backend is pluggable:

- **IPFS** (default): reports are added to a local IPFS node (Docker
  `ipfs/kubo`) and `off_chain_uri` becomes `ipfs://<CID>`. A SQLite index maps
  `report_id -> CID` so reports can be resolved back by id.
- **SQLite fallback**: if the IPFS node is unreachable the report payload is
  kept locally in SQLite instead (graceful degradation), so the pipeline stays
  testable without a running IPFS node.

`org_keys` (API key -> org) always lives in SQLite.
"""
from __future__ import annotations

import json
import os
import sqlite3
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional, Union


class StorageError(Exception):
    pass


class IpfsStore:
    """Minimal IPFS client for the HTTP RPC API (no external deps).

    Talks to a local IPFS node (e.g. `docker run -d -p 5001:5001 ipfs/kubo`).
    """

    def __init__(self, api_url: str = "http://localhost:5001/api/v0") -> None:
        self.api_url = api_url.rstrip("/")

    def _request(self, endpoint: str, timeout: int, data: Optional[bytes] = None) -> Any:
        req = urllib.request.Request(
            f"{self.api_url}/{endpoint}",
            data=data or b"",
            method="POST",  # kubo's HTTP RPC accepts POST; GET returns 405
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.loads(r.read().decode("utf-8"))
        except (urllib.error.URLError, OSError, json.JSONDecodeError) as exc:
            raise StorageError(f"IPFS {endpoint} failed: {exc}") from exc

    def is_available(self) -> bool:
        try:
            resp = self._request("version", timeout=2)
            return bool(resp.get("Version"))
        except StorageError:
            return False

    def add_bytes(self, data: bytes, filename: str = "report.json") -> str:
        """Add bytes to IPFS, returns the CID (v0)."""
        boundary = "----bMISINFO" + str(time.time_ns())
        body = (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
            "Content-Type: application/json\r\n\r\n"
        ).encode("utf-8") + data + f"\r\n--{boundary}--\r\n".encode("utf-8")
        req = urllib.request.Request(
            f"{self.api_url}/add",
            data=body,
            headers={"Content-Type": f"multipart/form-data; boundary={boundary}"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=15) as r:
                resp = json.loads(r.read().decode("utf-8"))
        except (urllib.error.URLError, OSError, json.JSONDecodeError) as exc:
            raise StorageError(f"IPFS add failed: {exc}") from exc
        cid = resp.get("Hash")
        if not cid:
            raise StorageError(f"IPFS add returned no CID: {resp}")
        return cid

    def cat_bytes(self, cid: str, timeout: int = 15) -> bytes:
        resp = self._request(f"cat?arg={cid}", timeout=timeout)
        if isinstance(resp, str):
            return resp.encode("utf-8")
        return json.dumps(resp).encode("utf-8")


class OffChainStore:
    """Off-chain store: IPFS for report payloads + SQLite index + org keys."""

    def __init__(
        self,
        db_path: str = "offchain.db",
        ipfs: Optional[Union[str, IpfsStore]] = None,
    ) -> None:
        parent = os.path.dirname(os.path.abspath(db_path))
        Path(parent).mkdir(parents=True, exist_ok=True)
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._init()

        if ipfs is None:
            ipfs = IpfsStore()
        self._ipfs = ipfs if isinstance(ipfs, IpfsStore) else IpfsStore(ipfs)
        self._ipfs_ok = self._ipfs.is_available()

    def _init(self) -> None:
        with self._conn:
            self._conn.execute(
                """
                CREATE TABLE IF NOT EXISTS org_keys (
                    api_key TEXT PRIMARY KEY,
                    org      TEXT NOT NULL
                )
                """
            )
            self._conn.execute(
                """
                CREATE TABLE IF NOT EXISTS reports (
                    report_id   TEXT PRIMARY KEY,
                    payload     TEXT NOT NULL,
                    content_hash TEXT NOT NULL,
                    created_at  TEXT NOT NULL
                )
                """
            )
            self._conn.execute(
                """
                CREATE TABLE IF NOT EXISTS report_index (
                    report_id   TEXT PRIMARY KEY,
                    cid         TEXT NOT NULL,
                    created_at  TEXT NOT NULL
                )
                """
            )

    # ---- IPFS status -------------------------------------------------------
    @property
    def using_ipfs(self) -> bool:
        return self._ipfs_ok

    def ipfs_status(self) -> Dict[str, Any]:
        return {"ipfs_available": self._ipfs_ok, "backend": "ipfs" if self._ipfs_ok else "sqlite"}

    # ---- org keys ----------------------------------------------------------
    def upsert_org_key(self, api_key: str, org: str) -> None:
        with self._conn:
            self._conn.execute(
                "INSERT OR REPLACE INTO org_keys (api_key, org) VALUES (?, ?)",
                (api_key, org),
            )

    def org_for_key(self, api_key: str) -> Optional[str]:
        row = self._conn.execute(
            "SELECT org FROM org_keys WHERE api_key=?", (api_key,)
        ).fetchone()
        return row["org"] if row else None

    # ---- reports -----------------------------------------------------------
    def save_report(self, report_id: str, report: Dict[str, Any]) -> str:
        """Store a report off-chain. Returns the off-chain URI.

        Prefers IPFS (cid://...) but degrades to a local SQLite payload if the
        IPFS node is unreachable.
        """
        payload_bytes = json.dumps(report, sort_keys=True).encode("utf-8")
        created_at = report.get("submitter", {}).get("submitted_at", "")
        cid = ""
        if self._ipfs_ok:
            try:
                cid = self._ipfs.add_bytes(payload_bytes)
                with self._conn:
                    self._conn.execute(
                        "INSERT OR REPLACE INTO report_index (report_id, cid, created_at) "
                        "VALUES (?, ?, ?)",
                        (report_id, cid, created_at),
                    )
                return f"ipfs://{cid}"
            except StorageError:
                self._ipfs_ok = False  # degrade for the rest of this process
                print(f"[storage] IPFS unreachable, falling back to SQLite for {report_id}")
        with self._conn:
            self._conn.execute(
                """
                INSERT OR REPLACE INTO reports (report_id, payload, content_hash, created_at)
                VALUES (?, ?, ?, ?)
                """,
                (report_id, payload_bytes.decode("utf-8"), report.get("content_hash", ""), created_at),
            )
        return f"http://localhost:8000/api/reports/{report_id}"

    def get_report(self, report_id: str) -> Optional[Dict[str, Any]]:
        """Fetch a stored report, resolving from IPFS if it was pinned there."""
        row = self._conn.execute(
            "SELECT cid FROM report_index WHERE report_id=?", (report_id,)
        ).fetchone()
        if row:
            try:
                return json.loads(self._ipfs.cat_bytes(row["cid"]).decode("utf-8"))
            except StorageError:
                # IPFS unreachable / content unpinned: fall back to local copy
                pass
        local = self._conn.execute(
            "SELECT payload FROM reports WHERE report_id=?", (report_id,)
        ).fetchone()
        return json.loads(local["payload"]) if local else None

    def list_report_ids(self) -> List[str]:
        rows = self._conn.execute("SELECT report_id FROM reports").fetchall()
        ipfs_rows = self._conn.execute("SELECT report_id FROM report_index").fetchall()
        ids = {r["report_id"] for r in rows} | {r["report_id"] for r in ipfs_rows}
        return sorted(ids)
