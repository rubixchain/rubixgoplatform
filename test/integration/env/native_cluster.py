"""
native_cluster.py — launch the 3-node Rubix cluster as native host processes.

This is the no-Docker counterpart to StressDockerManager (docker_env.py). Instead
of `docker compose up`, it runs three `rubixgoplatform` processes directly on the
host, each in its own working directory, so the suite can run on macOS / Windows
where Linux containers aren't available on CI.

Per node it reproduces what test/docker/rubix/entrypoint.sh does in bash, but in
cross-platform Python:
  1. create a working dir (the node's CWD — Rubix shells out to ./ipfs relative
     to CWD, see core/ipfs.go:41),
  2. drop the OS-correct kubo binary (ipfs / ipfs.exe) and localnetswarm.key into
     that dir,
  3. render config.toml with this node's node_index (0/1/2) and DB coordinates —
     every port (API/IPFS/swarm/DB) is derived from node_index in
     core/config/config.go:141-152, so distinct indices give non-colliding ports,
  4. `rubixgoplatform init -p <dir>` then `rubixgoplatform run -p <dir>` as a
     background process,
  5. health-check the node's HTTP API port until it answers.

teardown() terminates the processes cross-platform via the Popen handle (no
pkill / taskkill).

Port derivation (core/config/config.go + constants/ipfs.go), per node_index i:
    RubixServerPort = 20000 + i      (HTTP API the tests hit)
    SwarmPort       = 4002  + i      (IPFS swarm — distinct per node on one host)
    IPFSAPIPort     = 8081  + i
    IPFSPort        = 5002  + i
    Postgres        = 5433  + i      (matches native_postgres.py)
"""

from __future__ import annotations

import logging
import os
import platform
import shutil
import socket
import subprocess
import sys
import tarfile
import time
import urllib.request
import zipfile
from dataclasses import dataclass
from typing import List, Optional

log = logging.getLogger(__name__)

_IS_WINDOWS = os.name == "nt"

# Kubo version vendored for Docker (test/docker/rubix/kubo_v0.19.1_linux-amd64.tar.gz).
# Native mode fetches the matching host build on demand.
_KUBO_VERSION = "v0.19.1"
_KUBO_DIST = "https://dist.ipfs.tech/kubo/{ver}/kubo_{ver}_{plat}.{ext}"

# Swarm key shared by all localnet nodes (same file the Docker swarm mount uses).
_SWARM_KEY_SRC = os.path.join("test", "docker", "swarm", "localnetswarm.key")


@dataclass(frozen=True)
class NativeNode:
    """One node in the native cluster."""

    name: str        # nodeA / nodeB / quorum
    node_index: int  # 0 / 1 / 2 — drives ALL derived ports
    db_name: str     # rubix_a / rubix_b / rubix_q
    db_port: int     # 5433 / 5434 / 5435
    work_dir: str    # the node's CWD / -p data dir

    @property
    def api_port(self) -> int:
        return 20000 + self.node_index


class NativeClusterManager:
    """Build (config + assets), launch, health-check, and stop native nodes."""

    def __init__(
        self,
        nodes: List[NativeNode],
        binary: str,
        network_mode: str = "localnet",
        startup_timeout: int = 120,
    ) -> None:
        self._nodes = nodes
        self._binary = os.path.abspath(binary)
        self._network = network_mode
        self._startup_timeout = startup_timeout
        self._procs: dict[str, subprocess.Popen] = {}
        self._ipfs_bin: Optional[str] = None  # resolved lazily in up()

    # ----- lifecycle ------------------------------------------------------ #

    def up(self) -> None:
        if not os.path.isfile(self._binary):
            raise FileNotFoundError(
                f"rubixgoplatform binary not found at {self._binary}. "
                f"Build it first (e.g. `make compile-mac`)."
            )
        self._ipfs_bin = self._resolve_kubo_binary()

        for node in self._nodes:
            self._prepare_node_dir(node)
            self._init_node(node)
            self._render_config(node)
            self._launch_node(node)

        for node in self._nodes:
            self._wait_healthy(node)
        log.info("All %d native node(s) healthy.", len(self._nodes))

    def down(self) -> None:
        for name, proc in list(self._procs.items()):
            if proc.poll() is not None:
                continue
            log.info("[%s] terminating node process (pid %s)…", name, proc.pid)
            try:
                proc.terminate()
                try:
                    proc.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    log.warning("[%s] did not exit on terminate; killing.", name)
                    proc.kill()
                    proc.wait(timeout=10)
            except Exception as exc:  # noqa: BLE001
                log.warning("[%s] teardown error: %s", name, exc)
        self._procs.clear()

    # ----- per-node setup ------------------------------------------------- #

    def _prepare_node_dir(self, node: NativeNode) -> None:
        # Fresh dir every run — no stale .ipfs/, config.toml, or DB pointers.
        if os.path.isdir(node.work_dir):
            shutil.rmtree(node.work_dir, ignore_errors=True)
        os.makedirs(node.work_dir, exist_ok=True)

        # Place the kubo binary as ./ipfs (or ./ipfs.exe) in the node's CWD —
        # Rubix invokes it by relative path (core/ipfs.go:41-45).
        ipfs_name = "ipfs.exe" if _IS_WINDOWS else "ipfs"
        dest_ipfs = os.path.join(node.work_dir, ipfs_name)
        shutil.copy2(self._ipfs_bin, dest_ipfs)
        if not _IS_WINDOWS:
            os.chmod(dest_ipfs, 0o755)

        # Place the localnet swarm key in CWD — initIPFS copies it from there
        # (core/ipfs.go:89, entrypoint.sh:72).
        if not os.path.isfile(_SWARM_KEY_SRC):
            raise FileNotFoundError(f"swarm key not found at {_SWARM_KEY_SRC}")
        shutil.copy2(_SWARM_KEY_SRC, os.path.join(node.work_dir, "localnetswarm.key"))

    def _init_node(self, node: NativeNode) -> None:
        # `init` writes a default config.toml template into -p; we overwrite it
        # with the rendered per-node config next (mirrors the Docker entrypoint:
        # init, then envsubst the real config over the top).
        #
        # -p MUST be absolute: the node joins -p onto its CWD, and we set CWD to
        # work_dir (Rubix needs ./ipfs relative to CWD). A relative -p would
        # resolve to work_dir/work_dir and the node would read the wrong config.
        abs_dir = os.path.abspath(node.work_dir)
        log.info("[%s] rubixgoplatform init -p %s", node.name, abs_dir)
        subprocess.run(
            [self._binary, "init", "-p", abs_dir],
            check=True, cwd=node.work_dir, capture_output=True, text=True,
        )

    def _render_config(self, node: NativeNode) -> None:
        """Write config.toml with this node's node_index / network / DB coords.

        Python replacement for envsubst — works on Windows (no gettext). The
        node derives every port from node_index, so we only need to vary the
        index, network mode, and DB connection per node.
        """
        config = _NODE_CONFIG_TEMPLATE.format(
            node_index=node.node_index,
            network_mode=self._network,
            db_host="127.0.0.1",
            db_port=node.db_port,
            db_name=node.db_name,
        )
        # Write to the SAME absolute path the node reads (-p is absolute, see
        # _init_node), so this render — not init's template — is what `run` parses.
        cfg_path = os.path.join(os.path.abspath(node.work_dir), "config.toml")
        with open(cfg_path, "w", encoding="utf-8") as fh:
            fh.write(config)

    def _launch_node(self, node: NativeNode) -> None:
        log.info(
            "[%s] starting node (index=%d, api=%d, db=%d)…",
            node.name, node.node_index, node.api_port, node.db_port,
        )
        abs_dir = os.path.abspath(node.work_dir)
        logf = open(os.path.join(abs_dir, "node.log"), "w", encoding="utf-8")
        # CWD = work_dir is REQUIRED: Rubix runs ./ipfs and reads ./localnetswarm.key
        # relative to CWD (entrypoint.sh:6 comment). -p is absolute so the node
        # reads our rendered config, not a work_dir/work_dir duplicate.
        proc = subprocess.Popen(
            [self._binary, "run", "-p", abs_dir],
            cwd=abs_dir,
            stdout=logf,
            stderr=subprocess.STDOUT,
        )
        self._procs[node.name] = proc

    # ----- health --------------------------------------------------------- #

    def _wait_healthy(self, node: NativeNode) -> None:
        deadline = time.time() + self._startup_timeout
        while time.time() < deadline:
            proc = self._procs.get(node.name)
            if proc is not None and proc.poll() is not None:
                raise RuntimeError(
                    f"[{node.name}] node exited early (code {proc.returncode}). "
                    f"See {os.path.join(node.work_dir, 'node.log')}."
                )
            if self._tcp_open("127.0.0.1", node.api_port):
                log.info("[%s] API reachable on :%d", node.name, node.api_port)
                return
            time.sleep(1.0)
        raise TimeoutError(
            f"[{node.name}] API port {node.api_port} not reachable within "
            f"{self._startup_timeout}s. See {os.path.join(node.work_dir, 'node.log')}."
        )

    @staticmethod
    def _tcp_open(host: str, port: int) -> bool:
        try:
            with socket.create_connection((host, port), timeout=1):
                return True
        except OSError:
            return False

    # ----- kubo (ipfs) binary resolution ---------------------------------- #

    def _resolve_kubo_binary(self) -> str:
        """Return a path to a host-native kubo `ipfs` binary, downloading the
        pinned version into test/native/.kubo/ if not already cached."""
        plat = self._kubo_platform()
        ext = "zip" if _IS_WINDOWS else "tar.gz"
        cache_dir = os.path.join("test", "native", ".kubo", f"{_KUBO_VERSION}_{plat}")
        ipfs_name = "ipfs.exe" if _IS_WINDOWS else "ipfs"
        cached = os.path.join(cache_dir, ipfs_name)
        if os.path.isfile(cached):
            return cached

        os.makedirs(cache_dir, exist_ok=True)
        url = _KUBO_DIST.format(ver=_KUBO_VERSION, plat=plat, ext=ext)
        archive = os.path.join(cache_dir, f"kubo.{ext}")
        log.info("Downloading kubo %s (%s) …", _KUBO_VERSION, plat)
        urllib.request.urlretrieve(url, archive)  # noqa: S310 (pinned dist URL)

        if ext == "zip":
            with zipfile.ZipFile(archive) as zf:
                zf.extractall(cache_dir)
        else:
            with tarfile.open(archive) as tf:
                tf.extractall(cache_dir)  # noqa: S202 (trusted dist archive)
        extracted = os.path.join(cache_dir, "kubo", ipfs_name)
        if not os.path.isfile(extracted):
            raise FileNotFoundError(f"kubo archive did not contain {ipfs_name}")
        shutil.move(extracted, cached)
        if not _IS_WINDOWS:
            os.chmod(cached, 0o755)
        return cached

    @staticmethod
    def _kubo_platform() -> str:
        """Map the host to a kubo dist platform string (e.g. darwin-arm64)."""
        sysname = {"Darwin": "darwin", "Windows": "windows", "Linux": "linux"}.get(
            platform.system()
        )
        machine = platform.machine().lower()
        arch = "arm64" if machine in ("arm64", "aarch64") else "amd64"
        if sysname is None:
            raise RuntimeError(f"unsupported OS for native mode: {platform.system()}")
        return f"{sysname}-{arch}"


# config.toml the node parses from -p (mirrors test/docker/rubix/config.template.toml,
# but rendered in Python). Only node_index / network_mode / db coords vary per node;
# every port is derived from node_index by the node itself.
_NODE_CONFIG_TEMPLATE = """\
[core]
node_index = {node_index}
network_mode = "{network_mode}"

[db]
host = "{db_host}"
port = {db_port}
username = "rubix"
password = "rubixpass"
db_name = "{db_name}"

[db.config]
max_connections = 50
min_connections = 10
max_connection_lifetime_seconds = 60
max_connection_idletime_seconds = 30
statement_timeout_seconds = 20

[ipfs]
mainnet_bootstrap_nodes = []
testnet_bootstrap_nodes = []
localnet_bootstrap_nodes = []
"""
