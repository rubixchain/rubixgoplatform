"""
native_postgres.py — manage local PostgreSQL instances for native (no-Docker) mode.

The Docker harness runs each node's Postgres as a `postgres:16` container. Native
mode instead stands up real Postgres instances on the host, one per node, on the
ports the rubixgoplatform binary expects (constants.PostgresBasePort + node_index
=> 5433 / 5434 / 5435). Each instance has its own data dir, a `rubix` superuser
with password `rubixpass`, and a single database (rubix_a / rubix_b / rubix_q) —
matching test/docker/docker-compose.stress.yml and the node's config.template.toml.

Cross-platform by design (macOS / Windows / Linux):
  - locates initdb/pg_ctl from PATH or common install locations (Homebrew, EDB),
  - drives them via subprocess (no pg_isready / shell tools),
  - readiness is a TCP connect + a psycopg `SELECT 1`, not pg_isready.

NativePostgresManager owns the lifecycle; NativeEnvironment composes it with the
node-cluster manager.
"""

from __future__ import annotations

import logging
import os
import platform
import shutil
import socket
import subprocess
import time
from dataclasses import dataclass
from typing import List, Optional

log = logging.getLogger(__name__)

_IS_WINDOWS = os.name == "nt"

# Superuser created in each instance — must match config.template.toml [db].
_DB_USER = "rubix"
_DB_PASSWORD = "rubixpass"


@dataclass(frozen=True)
class PgInstance:
    """One Postgres instance: a data dir on a port, hosting one database."""

    name: str          # nodeA / nodeB / quorum (for logs)
    port: int          # 5433 / 5434 / 5435
    db_name: str       # rubix_a / rubix_b / rubix_q
    data_dir: str      # per-instance PGDATA


def _find_binary(name: str) -> str:
    """Locate a Postgres CLI binary (initdb / pg_ctl / postgres).

    Checks PATH first, then common per-OS install locations. Raises with an
    actionable message if not found, since native mode can't proceed without it.
    """
    exe = name + (".exe" if _IS_WINDOWS else "")
    found = shutil.which(exe)
    if found:
        return found

    candidates: List[str] = []
    system = platform.system()
    if system == "Darwin":
        # Homebrew (Apple Silicon + Intel) and the Postgres.app bundle.
        candidates += [
            f"/opt/homebrew/bin/{exe}",
            f"/usr/local/bin/{exe}",
        ]
        # Homebrew versioned kegs: /opt/homebrew/opt/postgresql@16/bin
        for base in ("/opt/homebrew/opt", "/usr/local/opt"):
            if os.path.isdir(base):
                for entry in sorted(os.listdir(base), reverse=True):
                    if entry.startswith("postgresql"):
                        candidates.append(os.path.join(base, entry, "bin", exe))
        candidates.append(
            f"/Applications/Postgres.app/Contents/Versions/latest/bin/{exe}"
        )
    elif system == "Windows":
        for base in (
            r"C:\Program Files\PostgreSQL",
            r"C:\Program Files (x86)\PostgreSQL",
        ):
            if os.path.isdir(base):
                for entry in sorted(os.listdir(base), reverse=True):  # newest major first
                    candidates.append(os.path.join(base, entry, "bin", exe))
    else:  # Linux
        candidates += [f"/usr/bin/{exe}", f"/usr/lib/postgresql/16/bin/{exe}"]
        base = "/usr/lib/postgresql"
        if os.path.isdir(base):
            for entry in sorted(os.listdir(base), reverse=True):
                candidates.append(os.path.join(base, entry, "bin", exe))

    for c in candidates:
        if os.path.isfile(c):
            return c

    raise FileNotFoundError(
        f"Could not find '{name}' on PATH or in common install locations. "
        f"Install PostgreSQL (e.g. `brew install postgresql@16` on macOS) and "
        f"ensure its bin/ is on PATH."
    )


class NativePostgresManager:
    """Stand up / tear down N local Postgres instances for the native cluster."""

    def __init__(self, instances: List[PgInstance], startup_timeout: int = 60) -> None:
        self._instances = instances
        self._startup_timeout = startup_timeout
        self._initdb = _find_binary("initdb")
        self._pg_ctl = _find_binary("pg_ctl")

    # ----- lifecycle ------------------------------------------------------ #

    def up(self) -> None:
        """initdb + start + create role/db for every instance, then block until
        each accepts a real query. Idempotent per run: data dirs are wiped first
        so state never leaks between runs (mirrors DockerEnvironment._clean_state)."""
        for inst in self._instances:
            self._init_instance(inst)
            self._start_instance(inst)
            self._wait_ready(inst)
            self._ensure_role_and_db(inst)
        log.info("All %d native Postgres instance(s) ready.", len(self._instances))

    def down(self) -> None:
        """Best-effort stop of every instance (safe to call on failure)."""
        for inst in self._instances:
            try:
                self._stop_instance(inst)
            except Exception as exc:  # noqa: BLE001
                log.warning("[%s] Postgres stop failed: %s", inst.name, exc)

    # ----- per-instance steps --------------------------------------------- #

    def _init_instance(self, inst: PgInstance) -> None:
        # Fresh data dir every run — no stale rubix_* state across runs.
        if os.path.isdir(inst.data_dir):
            shutil.rmtree(inst.data_dir, ignore_errors=True)
        # Create only the PARENT; let initdb create the leaf data dir itself.
        # initdb locks the data dir to 0700, and on Windows it can't change
        # permissions on a pre-existing directory ("could not change permissions
        # of directory: Permission denied"). When initdb creates the dir, it sets
        # the right perms at creation. Harmless on Unix too.
        os.makedirs(os.path.dirname(inst.data_dir), exist_ok=True)

        # Trust local connections so we can create the rubix role without a
        # bootstrap password dance; this is a throwaway test instance on loopback.
        log.info("[%s] initdb -> %s", inst.name, inst.data_dir)
        proc = subprocess.run(
            [self._initdb, "-D", inst.data_dir, "-U", "postgres",
             "--auth=trust", "--encoding=UTF8"],
            capture_output=True, text=True,
        )
        if proc.returncode != 0:
            # initdb's reason is in its stdout/stderr; capture_output hides it
            # unless we surface it. Persist alongside the data dir too so it
            # lands in the CI artifact.
            try:
                with open(os.path.join(os.path.dirname(inst.data_dir), "initdb.log"),
                          "w", encoding="utf-8") as fh:
                    fh.write((proc.stdout or "") + "\n--- stderr ---\n" + (proc.stderr or ""))
            except OSError:
                pass
            log.error(
                "[%s] initdb failed (rc=%s).\nstdout:\n%s\nstderr:\n%s",
                inst.name, proc.returncode, proc.stdout, proc.stderr,
            )
            raise subprocess.CalledProcessError(
                proc.returncode, proc.args, proc.stdout, proc.stderr
            )

    def _start_instance(self, inst: PgInstance) -> None:
        logfile = os.path.join(inst.data_dir, "server.log")
        # Bind loopback only; set the port via -o so each instance is isolated.
        opts = f"-p {inst.port} -h 127.0.0.1"
        # On Unix, override the unix-socket dir to the (writable) data dir. The
        # default (/var/run/postgresql) is NOT writable by the non-root CI runner
        # user, so `pg_ctl start` fails there. Use an absolute path — pg requires
        # it for -k. Windows has no unix sockets, so skip.
        if not _IS_WINDOWS:
            opts += f" -k {os.path.abspath(inst.data_dir)}"
        log.info("[%s] starting Postgres on 127.0.0.1:%d", inst.name, inst.port)
        try:
            subprocess.run(
                [self._pg_ctl, "-D", inst.data_dir, "-l", logfile,
                 "-o", opts, "-w", "-t", str(self._startup_timeout), "start"],
                check=True, capture_output=True, text=True,
            )
        except subprocess.CalledProcessError as exc:
            # pg_ctl's own stderr rarely says why; the reason is in server.log.
            # Surface both so a CI failure is diagnosable without the artifact.
            server_log = ""
            try:
                with open(logfile, encoding="utf-8") as fh:
                    server_log = fh.read()[-2000:]
            except OSError:
                pass
            log.error(
                "[%s] pg_ctl start failed (rc=%s).\nstdout:%s\nstderr:%s\nserver.log:\n%s",
                inst.name, exc.returncode, exc.stdout, exc.stderr, server_log,
            )
            raise

    def _stop_instance(self, inst: PgInstance) -> None:
        if not os.path.isdir(inst.data_dir):
            return
        log.info("[%s] stopping Postgres (port %d)", inst.name, inst.port)
        subprocess.run(
            [self._pg_ctl, "-D", inst.data_dir, "-m", "fast", "-w", "stop"],
            check=False, capture_output=True, text=True,
        )

    # ----- readiness + provisioning --------------------------------------- #

    def _wait_ready(self, inst: PgInstance) -> None:
        deadline = time.time() + self._startup_timeout
        last_err: Optional[Exception] = None
        while time.time() < deadline:
            if self._tcp_open("127.0.0.1", inst.port) and self._query_ok(inst, "postgres"):
                return
            time.sleep(0.5)
        raise TimeoutError(
            f"[{inst.name}] Postgres on port {inst.port} not ready within "
            f"{self._startup_timeout}s (last error: {last_err})"
        )

    def _ensure_role_and_db(self, inst: PgInstance) -> None:
        """Create the rubix superuser and the node's database (idempotent)."""
        import psycopg2  # local import: only needed in native mode

        conn = psycopg2.connect(
            host="127.0.0.1", port=inst.port, user="postgres", dbname="postgres"
        )
        try:
            conn.autocommit = True
            with conn.cursor() as cur:
                cur.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", (_DB_USER,))
                if not cur.fetchone():
                    cur.execute(
                        f"CREATE ROLE {_DB_USER} LOGIN SUPERUSER PASSWORD %s",
                        (_DB_PASSWORD,),
                    )
                cur.execute(
                    "SELECT 1 FROM pg_database WHERE datname = %s", (inst.db_name,)
                )
                if not cur.fetchone():
                    # CREATE DATABASE can't run in a parameterized/transactional
                    # form; identifier is internal (rubix_a/b/q), not user input.
                    cur.execute(f"CREATE DATABASE {inst.db_name} OWNER {_DB_USER}")
            log.info("[%s] role '%s' + db '%s' ready.", inst.name, _DB_USER, inst.db_name)
        finally:
            conn.close()

    # ----- low-level probes ----------------------------------------------- #

    @staticmethod
    def _tcp_open(host: str, port: int) -> bool:
        try:
            with socket.create_connection((host, port), timeout=1):
                return True
        except OSError:
            return False

    @staticmethod
    def _query_ok(inst: PgInstance, dbname: str) -> bool:
        try:
            import psycopg2
            conn = psycopg2.connect(
                host="127.0.0.1", port=inst.port, user="postgres",
                dbname=dbname, connect_timeout=2,
            )
            try:
                with conn.cursor() as cur:
                    cur.execute("SELECT 1")
                    cur.fetchone()
                return True
            finally:
                conn.close()
        except Exception:  # noqa: BLE001
            return False
