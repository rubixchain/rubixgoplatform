"""callback_receiver.py — Lightweight host-side HTTP receiver used to verify
that Rubix node subscribers actually POST to the URL registered in the
`call_back_urls` table when a smart contract is executed.

Usage (programmatic):

    from test.integration.engines.callback_receiver import CallbackReceiver
    rx = CallbackReceiver()                # starts on a free port
    rx.start()
    print(rx.url_for_docker())             # -> http://host.docker.internal:<port>/cb
    ...
    captured = rx.wait_for_any(timeout=15) # blocks until at least 1 POST lands
    rx.stop()

The receiver accepts any path starting with '/cb'. Response body is always
`{"message":"ok"}` so nodes can cleanly parse it (ContractCallBack expects a
JSON object with a "message" string field).
"""

from __future__ import annotations

import json
import logging
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any, Dict, List, Optional

log = logging.getLogger(__name__)


class _Handler(BaseHTTPRequestHandler):
    server_version = "RubixCallbackRx/1.0"

    def _reply(self, code: int, body: Dict[str, Any]) -> None:
        payload = json.dumps(body).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt: str, *args: Any) -> None:  # noqa: D401
        # Route BaseHTTPRequestHandler's stderr logging into our logger.
        log.debug("callback_rx %s - %s", self.client_address[0], fmt % args)

    def do_POST(self) -> None:  # noqa: N802
        length = int(self.headers.get("Content-Length", "0") or "0")
        raw = self.rfile.read(length) if length > 0 else b""
        body: Any
        try:
            body = json.loads(raw.decode("utf-8")) if raw else {}
        except Exception:  # noqa: BLE001
            body = {"_raw": raw.decode("utf-8", errors="replace")}

        event = {
            "received_at": time.time(),
            "path": self.path,
            "headers": {k: v for k, v in self.headers.items()},
            "body": body,
            "client": f"{self.client_address[0]}:{self.client_address[1]}",
        }
        rx: "CallbackReceiver" = self.server.rx  # type: ignore[attr-defined]
        with rx._lock:
            rx._events.append(event)
            rx._event_ready.notify_all()
        self._reply(200, {"message": "ok"})

    def do_GET(self) -> None:  # noqa: N802
        # Health-check endpoint so callers can sanity-check the receiver is up.
        self._reply(200, {"message": "alive"})


class CallbackReceiver:
    """A host-side HTTP receiver that captures POSTs from in-container nodes."""

    def __init__(self, host: str = "0.0.0.0", port: int = 0, path_prefix: str = "/cb"):
        self.host = host
        self.port = port
        self.path_prefix = path_prefix
        self._server: Optional[ThreadingHTTPServer] = None
        self._thread: Optional[threading.Thread] = None
        self._events: List[Dict[str, Any]] = []
        self._lock = threading.Lock()
        self._event_ready = threading.Condition(self._lock)

    # --------------------------------------------------------------- lifecycle

    def start(self) -> None:
        if self._server is not None:
            return
        srv = ThreadingHTTPServer((self.host, self.port), _Handler)
        srv.rx = self  # type: ignore[attr-defined]
        self.port = srv.server_address[1]
        self._server = srv
        self._thread = threading.Thread(
            target=srv.serve_forever, name="CallbackReceiver", daemon=True,
        )
        self._thread.start()
        log.info("callback receiver listening on %s:%d%s", self.host, self.port, self.path_prefix)

    def stop(self) -> None:
        if self._server is None:
            return
        self._server.shutdown()
        self._server.server_close()
        self._server = None
        if self._thread is not None:
            self._thread.join(timeout=2)
            self._thread = None

    # --------------------------------------------------------------- URL helpers

    def url_for_host(self) -> str:
        """URL usable from the host machine itself (e.g. local dev / pytest)."""
        return f"http://127.0.0.1:{self.port}{self.path_prefix}"

    def url_for_docker(self) -> str:
        """URL usable from inside a container on Docker Desktop (mac/win).

        Relies on the standard `host.docker.internal` alias. If you run this on
        Linux without Docker Desktop, pass the host's LAN IP instead.
        """
        return f"http://host.docker.internal:{self.port}{self.path_prefix}"

    # --------------------------------------------------------------- inspection

    def captured(self) -> List[Dict[str, Any]]:
        with self._lock:
            return list(self._events)

    def clear(self) -> None:
        with self._lock:
            self._events.clear()

    def wait_for_any(self, timeout: float = 15.0) -> List[Dict[str, Any]]:
        """Block until at least one event has been captured (or timeout)."""
        deadline = time.time() + timeout
        with self._event_ready:
            while not self._events:
                remaining = deadline - time.time()
                if remaining <= 0:
                    break
                self._event_ready.wait(timeout=remaining)
            return list(self._events)

    def wait_for_match(
        self, predicate, timeout: float = 15.0,
    ) -> Optional[Dict[str, Any]]:
        """Block until an event matches `predicate(event) -> bool` or timeout."""
        deadline = time.time() + timeout
        with self._event_ready:
            while True:
                for ev in self._events:
                    if predicate(ev):
                        return ev
                remaining = deadline - time.time()
                if remaining <= 0:
                    return None
                self._event_ready.wait(timeout=remaining)


# --------------------------------------------------------------------------- CLI

if __name__ == "__main__":  # pragma: no cover
    import argparse
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    ap = argparse.ArgumentParser(description="Run a standalone Rubix callback receiver.")
    ap.add_argument("--host", default="0.0.0.0")
    ap.add_argument("--port", type=int, default=0)
    args = ap.parse_args()
    rx = CallbackReceiver(host=args.host, port=args.port)
    rx.start()
    print(f"host URL   : {rx.url_for_host()}")
    print(f"docker URL : {rx.url_for_docker()}")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        rx.stop()
