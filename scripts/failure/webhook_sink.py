"""Webhook receiver for the failure drill (scripts/failure-drill.sh).

Verifies every delivery's signature (HMAC-SHA256 over "id.timestamp.body" with the endpoint
secret from WEBHOOK_SECRET), records webhook IDs, and answers GET /stats with counts. Runs on
the compose network next to the worker; the standard library only.
"""

import hashlib
import hmac
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SECRET = os.environ["WEBHOOK_SECRET"].encode()
LOCK = threading.Lock()
SEEN: dict[str, int] = {}
STATS = {"requests": 0, "bad_signature": 0}


def valid(headers, body: bytes) -> bool:
    msg = f"{headers.get('RiskOS-Webhook-Id', '')}.{headers.get('RiskOS-Webhook-Timestamp', '')}.".encode() + body
    want = "v1=" + hmac.new(SECRET, msg, hashlib.sha256).hexdigest()
    return any(hmac.compare_digest(want, s.strip()) for s in headers.get("RiskOS-Signature", "").split(","))


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with LOCK:
            STATS["requests"] += 1
            if not valid(self.headers, body):
                STATS["bad_signature"] += 1
                self.send_response(401)
                self.end_headers()
                return
            wid = self.headers.get("RiskOS-Webhook-Id", "")
            SEEN[wid] = SEEN.get(wid, 0) + 1
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        with LOCK:
            out = dict(STATS, unique=len(SEEN), duplicates=sum(n - 1 for n in SEEN.values()), ids=sorted(SEEN))
        body = json.dumps(out).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


ThreadingHTTPServer(("0.0.0.0", 9099), Handler).serve_forever()
