"""
docker_env.py — Docker Compose lifecycle for the Rubix integration test stack.

Uses test/docker/docker-compose.stress.yml (project rubix-stress).
Container names: rubix-stress-nodeA, rubix-stress-nodeB, rubix-stress-quorum,
and — only under the `fullnode` compose profile — rubix-stress-fullnode.

StressDockerManager is the low-level compose wrapper (up/down/health).
DockerEnvironment adapts it to the Environment interface (setup/teardown) so
the harness can swap in an external/non-Docker environment later without the
test suites changing.
"""

from __future__ import annotations

import logging
import os
import shutil
import subprocess
import time
from typing import List, Optional, Tuple

from test.integration.env.base import Environment

log = logging.getLogger(__name__)

NODE_PORTS = [20010, 20011, 20012]

# Compose profile that gates the optional 4th node (see docker-compose.stress.yml).
# Without it, `up` brings up exactly the 3-node stack; with it, the -fullnode
# node and its Postgres join the same bridge network.
FULLNODE_PROFILE = "fullnode"
FULLNODE_CONTAINER = "rubix-stress-fullnode"
FULLNODE_PORT = 20013

# Bind-mount state dir for the stress stack. The compose file mounts
# ./data/stress/<node>/{db,app} into the containers — these are BIND-MOUNTS,
# so `docker compose down -v` does NOT remove them. Stale state here (e.g.
# accumulated quorum DIDs in quorum_manager) causes "Quorum is not setup"
# pledge failures on the next run. We wipe it before each bring-up to guarantee
# isolation. Relative to repo root (where the runner is invoked).
_STATE_DIR = os.path.join("test", "docker", "data", "stress")

_CONTAINER_BY_PORT = {
    20010: "rubix-stress-nodeA",
    20011: "rubix-stress-nodeB",
    20012: "rubix-stress-quorum",
    FULLNODE_PORT: FULLNODE_CONTAINER,
}


class StressDockerManager:
    def __init__(
        self,
        compose_file: str = "test/docker/docker-compose.stress.yml",
        project_name: str = "rubix-stress",
        profiles: Optional[List[str]] = None,
    ) -> None:
        self.compose_file = compose_file
        self.project_name = project_name
        # Compose profiles to activate on every compose invocation. Empty =
        # the default 3-node stack. ["fullnode"] additionally starts the
        # -fullnode node + its Postgres.
        self.profiles: List[str] = list(profiles or [])

    def compose_argv(self, *args: str) -> List[str]:
        """Build a `docker compose` argv with this manager's file/project/profiles.

        Profiles must precede the subcommand, and they have to be repeated on
        EVERY invocation (up, logs, ps, down) — a profiled service is invisible
        to a command that doesn't name its profile.
        """
        cmd = ["docker", "compose", "-f", self.compose_file, "-p", self.project_name]
        for profile in self.profiles:
            cmd += ["--profile", profile]
        return cmd + list(args)

    def up(self, build: bool = True, timeout: int = 480) -> None:
        """Start the stack and block until all nodes are healthy.

        Timeout is larger than a standard stack because large token minting
        operations may push container startup longer.
        """
        ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        log.info(
            "[%s] Starting integration compose stack (build=%s, wait-timeout=%ds)…",
            ts, build, timeout,
        )
        self._force_remove_stale()

        # Build the shared image via a SINGLE service, not the whole project.
        # All three node services declare the same image tag (rubix-node:latest).
        # On a cold cache, `docker compose build` (or `up --build`) builds all
        # three in parallel and they race to export to the same tag — one fails
        # with `image "rubix-node:latest": already exists`. Building just one
        # service populates the tag once; `up` then reuses it for all three.
        if build:
            subprocess.run(self.compose_argv("build", "nodeA"), check=True)

        try:
            subprocess.run(
                self.compose_argv("up", "-d", "--wait", "--remove-orphans"),
                check=True,
            )
        except subprocess.CalledProcessError:
            # `up --wait` failed (a container is unhealthy or exited). Capture
            # container status + logs NOW, before anything tears the stack down —
            # the runner's finally:teardown() removes the containers, so a later
            # workflow step would find nothing. This is the only reliable place
            # to surface WHY a node didn't come up.
            self._dump_diagnostics()
            raise

        log.info("All integration nodes healthy.")

    def _dump_diagnostics(self) -> None:
        """Dump `compose ps -a` + `compose logs` to stdout and a log file.

        Called on bring-up failure, before teardown, so a crashed/exited node is
        diagnosable from CI artifacts (and the console).
        """
        log.error("Bring-up failed — capturing container diagnostics:")
        out_lines = []
        for label, args in (
            ("docker compose ps -a", ["ps", "-a"]),
            ("docker compose logs (tail 400)", ["logs", "--no-color", "--tail", "400"]),
        ):
            header = f"===== {label} ====="
            out_lines.append(header)
            log.error(header)
            try:
                res = subprocess.run(
                    self.compose_argv(*args),
                    capture_output=True, text=True, timeout=120,
                )
                body = (res.stdout or "") + (res.stderr or "")
            except Exception as exc:  # noqa: BLE001
                body = f"(failed to capture: {exc})"
            out_lines.append(body)
            for line in body.splitlines():
                log.error("  %s", line)
        # Persist alongside the run logs so it lands in the uploaded artifact.
        try:
            logs_dir = os.path.join(_STATE_DIR, "logs")
            os.makedirs(logs_dir, exist_ok=True)
            with open(os.path.join(logs_dir, "_bringup_diagnostics.txt"),
                      "w", encoding="utf-8") as fh:
                fh.write("\n".join(out_lines))
        except Exception as exc:  # noqa: BLE001
            log.error("Could not write _bringup_diagnostics.txt: %s", exc)

    def down(self, volumes: bool = True) -> None:
        ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        log.info("[%s] Tearing down integration stack (volumes=%s)…", ts, volumes)
        cmd = self.compose_argv("down")
        if volumes:
            cmd.append("-v")
        subprocess.run(cmd, check=True)
        log.info("Integration stack torn down.")

    def is_healthy_container(self, container: str) -> bool:
        """TCP-probe the node API inside *container* (same probe as the healthcheck)."""
        try:
            result = subprocess.run(
                ["docker", "exec", container, "bash", "-c", "</dev/tcp/localhost/20000"],
                capture_output=True,
                timeout=5,
            )
            return result.returncode == 0
        except Exception:
            return False

    def is_healthy(self, port: int) -> bool:
        container = _CONTAINER_BY_PORT.get(port, f"{self.project_name}-nodeA")
        try:
            result = subprocess.run(
                ["docker", "exec", container, "bash", "-c", "</dev/tcp/localhost/20000"],
                capture_output=True,
                timeout=5,
            )
            return result.returncode == 0
        except Exception:
            return False

    def _force_remove_stale(self) -> None:
        result = subprocess.run(
            ["docker", "ps", "-aq", "--filter", f"name={self.project_name}"],
            capture_output=True,
            text=True,
        )
        ids = result.stdout.strip().splitlines()
        if ids:
            log.info("Removing %d stale container(s) before up…", len(ids))
            subprocess.run(["docker", "rm", "-f"] + ids, check=False)


class ContainerController:
    """Docker-exec / logs / restart access to one container in the stack.

    The fullnode checks need three things a plain HTTP client cannot give them:
    the node's IPFS peer ID and gossipsub state (`ipfs` CLI inside the
    container), its stdout log (to assert the subscribe happened), and a
    restart (to assert re-ingestion is idempotent). Keeping this behind a small
    object means FullnodeEngine depends on a capability, not on Docker — a
    future native fullnode can supply its own implementation.
    """

    def __init__(self, container: str, dm: "StressDockerManager", service: str) -> None:
        self.container = container
        self._dm = dm
        self._service = service

    def exec(self, *args: str, timeout: int = 60) -> Tuple[int, str, str]:
        """Run a command inside the container. Returns (rc, stdout, stderr)."""
        try:
            res = subprocess.run(
                ["docker", "exec", self.container, *args],
                capture_output=True, text=True, timeout=timeout,
            )
            return res.returncode, res.stdout.strip(), res.stderr.strip()
        except Exception as exc:  # noqa: BLE001
            return 1, "", f"docker exec failed: {exc}"

    def ipfs(self, *args: str, timeout: int = 60) -> Tuple[int, str, str]:
        """Run the container's bundled kubo binary (`/app/ipfs`).

        The node rewrote kubo's API port to constants.IPFSPort (5002) at init
        (core/ipfs.go), and every container runs node_index 0, so the daemon
        inside each container answers on 127.0.0.1:5002.
        """
        return self.exec("/app/ipfs", "--api", "/ip4/127.0.0.1/tcp/5002",
                         *args, timeout=timeout)

    #: Where the node writes its structured log inside the container. `run`
    #: defaults -logFile to <nodeConfigPath>/log.txt (command/command.go:571),
    #: and entrypoint.sh passes -p /app/data — so the interesting lines
    #: ("Successfully subscribed to topic: rubix_txn", per-transaction
    #: validate/persist messages) are NOT on the container's stdout, which only
    #: carries the entrypoint echoes and kubo's daemon output.
    NODE_LOG_PATH = "/app/data/log.txt"

    def logs(self, tail: int = 2000) -> str:
        """Return the container's stdout/stderr AND the node's own log file.

        Both are needed: stdout shows startup/crash output from the entrypoint
        and IPFS, while log.txt is where every Core log line actually lands.
        """
        parts: List[str] = []
        try:
            res = subprocess.run(
                ["docker", "logs", "--tail", str(tail), self.container],
                capture_output=True, text=True, timeout=120,
            )
            parts.append("===== container stdout/stderr =====")
            parts.append((res.stdout or "") + (res.stderr or ""))
        except Exception as exc:  # noqa: BLE001
            parts.append(f"(could not read container logs: {exc})")

        rc, out, err = self.exec(
            "sh", "-c", f"tail -n {tail} {self.NODE_LOG_PATH} 2>/dev/null", timeout=120
        )
        parts.append(f"===== {self.NODE_LOG_PATH} (tail {tail}) =====")
        parts.append(out if rc == 0 and out else f"(unavailable: {err or 'no output'})")
        return "\n".join(parts)

    def node_log_contains(self, needle: str) -> bool:
        """Grep the node's WHOLE log file for a fixed string, inside the container.

        Not a substring search over logs(): startup lines such as the pubsub
        subscribe confirmation are written in the first seconds, while the file
        grows into the megabytes over a run — any tail-based search would miss
        them. grep -F -q -m1 short-circuits on the first hit, so this stays
        cheap even on a large log.
        """
        rc, _, _ = self.exec(
            "sh", "-c",
            f"grep -F -q -m1 -- \"$0\" {self.NODE_LOG_PATH} 2>/dev/null",
            needle,
            timeout=60,
        )
        return rc == 0

    def restart(self, wait_healthy_timeout: int = 180) -> None:
        """Restart the container and block until compose reports it healthy."""
        log.info("[%s] restarting container…", self.container)
        subprocess.run(
            self._dm.compose_argv("restart", self._service), check=True,
        )
        deadline = time.time() + wait_healthy_timeout
        while time.time() < deadline:
            if self._dm.is_healthy_container(self.container):
                log.info("[%s] healthy again after restart.", self.container)
                return
            time.sleep(2.0)
        raise TimeoutError(
            f"[{self.container}] did not become healthy within "
            f"{wait_healthy_timeout}s of restart."
        )


class DockerEnvironment(Environment):
    """Environment backed by a fresh docker-compose stack.

    Args:
        compose_file: path to the compose file (default: stress stack).
        project_name: compose project name (isolates containers/networks).
        build:        rebuild node images on up (default: True).
        teardown_volumes: remove volumes on teardown (default: True — clean DB
                          state every run).
        with_fullnode: also start the profile-gated `-fullnode` node + its
                       Postgres (see docker-compose.stress.yml). Off by default
                       so the standard suite's stack is unchanged.
    """

    NODE_PORTS: List[int] = NODE_PORTS

    def __init__(
        self,
        compose_file: str = "test/docker/docker-compose.stress.yml",
        project_name: str = "rubix-stress",
        build: bool = True,
        teardown_volumes: bool = True,
        with_fullnode: bool = False,
    ) -> None:
        self._dm = StressDockerManager(
            compose_file,
            project_name,
            profiles=[FULLNODE_PROFILE] if with_fullnode else [],
        )
        self._build = build
        self._teardown_volumes = teardown_volumes
        self.with_fullnode = with_fullnode

    def fullnode_controller(self) -> Optional[ContainerController]:
        """Return a controller for the fullnode container, or None if not started."""
        if not self.with_fullnode:
            return None
        return ContainerController(FULLNODE_CONTAINER, self._dm, "fullnode")

    def controller(self, service: str, container: str) -> ContainerController:
        return ContainerController(container, self._dm, service)

    def setup(self) -> None:
        self._clean_state()
        self._dm.up(build=self._build)

    def teardown(self) -> None:
        self._dm.down(volumes=self._teardown_volumes)

    @staticmethod
    def _clean_state() -> None:
        """Wipe the bind-mount state dir so every run starts from a clean DB.

        Critical for test isolation: the Postgres data is bind-mounted, so
        `down -v` leaves it behind. Without this, quorum DIDs and tokens from
        prior runs leak in and cause "Quorum is not setup" pledge failures.
        Preserves the logs/ subdir (run artifacts), wipes per-node db/app state.
        """
        if not os.path.isdir(_STATE_DIR):
            return
        for entry in os.listdir(_STATE_DIR):
            if entry == "logs":
                continue  # keep run artifacts (transactions.jsonl, summary.json, …)
            path = os.path.join(_STATE_DIR, entry)
            try:
                if os.path.isdir(path):
                    shutil.rmtree(path)
                else:
                    os.remove(path)
            except OSError as exc:  # noqa: BLE001
                # The containers write these bind-mounts as root, so a non-root
                # host user cannot remove them directly. Swallowing that leaves
                # the very stale state this method exists to prevent (and, since
                # the repo root is the image build context, an unreadable dir
                # also breaks the next `compose build`). Escalate to a
                # throwaway root container instead of continuing quietly.
                log.warning("Could not remove stale state %s directly (%s); "
                            "retrying as root in a container.", path, exc)
                if not DockerEnvironment._root_wipe(path):
                    raise RuntimeError(
                        f"Could not remove stale harness state at {path!r}. "
                        f"The next run would inherit it (stale quorum DIDs and "
                        f"tokens cause 'Quorum is not setup' pledge failures) and "
                        f"the unreadable directory would also break the image "
                        f"build context. Remove it manually: "
                        f"sudo rm -rf {path!r}"
                    ) from exc
        log.info("Cleaned stale bind-mount state under %s (kept logs/).", _STATE_DIR)

    @staticmethod
    def _root_wipe(path: str) -> bool:
        """Delete *path* from inside a throwaway root container.

        Mounts the PARENT directory and removes only the named child, so a bad
        path can never escape the harness state dir. Returns True on success.
        """
        parent = os.path.abspath(os.path.dirname(path))
        name = os.path.basename(path)
        if not name or name in (".", ".."):
            return False
        try:
            res = subprocess.run(
                ["docker", "run", "--rm", "-v", f"{parent}:/wipe",
                 "alpine:latest", "rm", "-rf", f"/wipe/{name}"],
                capture_output=True, text=True, timeout=120,
            )
            if res.returncode != 0:
                log.error("Root wipe of %s failed: %s", path,
                          (res.stderr or res.stdout).strip())
                return False
        except Exception as exc:  # noqa: BLE001
            log.error("Root wipe of %s failed: %s", path, exc)
            return False
        return not os.path.exists(path)
