"""
runner.py — Entry point for the Rubix integration test harness.

Owns three things the suite (tests/happy_path.py) deliberately does not:
  1. Argument parsing (CI-relevant flags only).
  2. The cluster Environment lifecycle (Docker compose up/down via env/).
  3. Process exit code — a failed verification check OR an exception maps to a
     non-zero exit so CI catches failures. (The suite logs failed checks but
     does not raise; we read runner.verification_failed here.)

Usage (from repo root):
  python3 -m test.integration.runner --run-all-tests
  python3 -m test.integration.runner --micro-test --run-all-tests   # fast smoke
  python3 -m test.integration.runner --small-test --run-all-tests
  python3 -m test.integration.runner --micro-test --nft-only --nft-count 5
  python3 -m test.integration.runner --no-docker --run-all-tests     # nodes already up
  python3 -m test.integration.runner --small-test --run-all-tests --fullnode-test

Scope: this committed harness intentionally omits the scratchpad-only resume
machinery (--no-teardown / --skip-setup / --skip-mint / --index-offset /
parallel-stack flags) from the local e2e_stress runner. CI always runs a clean
stack from scratch.
"""

from __future__ import annotations

import argparse
import logging
import sys

from test.integration.config import StressConfig
from test.integration.env.docker_env import DockerEnvironment
from test.integration.tests.happy_path import StressRunner

log = logging.getLogger(__name__)


def _build_arg_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="Rubix integration test runner",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument(
        "--config",
        metavar="PATH",
        default=None,
        help="Path to JSON config file (default: test/integration/config.json)",
    )
    p.add_argument(
        "--small-test",
        action="store_true",
        help="Apply small_test_overrides from config (quick validation run)",
    )
    p.add_argument(
        "--micro-test",
        action="store_true",
        help="Apply micro_test_overrides from config (minimal token counts, fast smoke)",
    )
    p.add_argument(
        "--no-docker",
        action="store_true",
        help="Skip docker up/down (nodes already running — e.g. external environment)",
    )
    p.add_argument(
        "--native",
        action="store_true",
        help="Run the cluster as native host processes (no Docker): local Postgres "
             "+ rubixgoplatform binaries. For macOS/Windows. Use with "
             "--config test/integration/config.native.json. Mutually exclusive with --no-docker.",
    )
    p.add_argument(
        "--max-transactions",
        type=int,
        default=None,
        metavar="N",
        help="Override max_transactions from config (e.g. --max-transactions 2000).",
    )
    # ------------------------------------------------------------------
    # NFT flags
    # ------------------------------------------------------------------
    p.add_argument(
        "--nft-count",
        type=int,
        default=0,
        metavar="N",
        help="Number of NFTs to create and deploy (default: 0, disabled).",
    )
    p.add_argument(
        "--nft-only",
        action="store_true",
        help="Skip RBT shuttle and only run NFT creation & deployment. Requires --nft-count > 0.",
    )
    p.add_argument(
        "--nft-self-execute",
        action="store_true",
        help="After NFT deployment, self-execute one NFT (no ownership transfer).",
    )
    p.add_argument(
        "--nft-transfer",
        action="store_true",
        help="After NFT deployment, execute one NFT and transfer ownership to the opposite node.",
    )
    p.add_argument(
        "--nft-cross-execute",
        action="store_true",
        help="After NFT deployment, subscribe and execute NFTs from the opposite node.",
    )
    # ------------------------------------------------------------------
    # Smart contract flags
    # ------------------------------------------------------------------
    p.add_argument(
        "--sc-count",
        type=int,
        default=0,
        help="Number of smart contracts to deploy from nodeA (default: 0).",
    )
    p.add_argument(
        "--sc-execute",
        action="store_true",
        help="Execute deployed smart contracts from nodeB (cross-node). Requires --sc-count > 0.",
    )
    p.add_argument(
        "--sc-only",
        action="store_true",
        help="Skip RBT shuttle and only run smart contract deploy/execute. Requires --sc-count > 0.",
    )
    p.add_argument(
        "--exec-rounds",
        type=int,
        default=0,
        metavar="N",
        help="Repeated execution rounds per deployed NFT/SC (default: 0). Alternates self/cross.",
    )
    p.add_argument(
        "--full-token-test",
        action="store_true",
        help=(
            "Run comprehensive NFT + SC lifecycle. Auto-sets --nft-count 2, "
            "--nft-self-execute, --nft-transfer, --nft-cross-execute, --sc-count 2, --sc-execute."
        ),
    )
    p.add_argument(
        "--bundled-test",
        action="store_true",
        help=(
            "Single /rubix/v1/tx call combining RBT + NFT + SC execution. "
            "Auto-sets --nft-count 2, --sc-count 2, --sc-execute. Alternates A->B / B->A."
        ),
    )
    p.add_argument(
        "--bundled-rounds",
        type=int,
        default=3,
        metavar="N",
        help="Number of bundled transaction rounds (default: 3).",
    )
    p.add_argument(
        "--bundled-rbt",
        type=float,
        default=1.0,
        metavar="AMT",
        help="RBT amount per bundled transaction round (default: 1.0).",
    )
    p.add_argument(
        "--decimal-transfers",
        action="store_true",
        help="Use random decimal transfer amounts (up to 3 dp, min 0.001 RBT) in shuttle phases.",
    )
    # ------------------------------------------------------------------
    # FT (Fungible Token) flags
    # ------------------------------------------------------------------
    p.add_argument(
        "--ft-count",
        type=int,
        default=0,
        metavar="N",
        help="Number of FT mint operations (default: 0, disabled). Alternates nodeA/nodeB.",
    )
    p.add_argument(
        "--ft-only",
        action="store_true",
        help="Skip RBT shuttle and only run FT mint/transfer. Requires --ft-count > 0.",
    )
    p.add_argument(
        "--ft-transfer",
        action="store_true",
        help="After FT minting, transfer a slice of each batch to the opposite node.",
    )
    p.add_argument(
        "--ft-transfer-rounds",
        type=int,
        default=0,
        metavar="N",
        help="Additional repeated FT transfer rounds per batch, alternating A<->B (default: 0).",
    )
    p.add_argument(
        "--ft-tokens-per-batch",
        type=int,
        default=100,
        metavar="N",
        help="Number of FTs created per mint operation (default: 100).",
    )
    p.add_argument(
        "--ft-rbt-per-batch",
        type=int,
        default=10,
        metavar="N",
        help="RBT burned per mint operation (default: 10).",
    )
    # ------------------------------------------------------------------
    # All-in-one transaction flags (RBT + FT[] + NFT[] + SC[] atomically)
    # ------------------------------------------------------------------
    p.add_argument(
        "--all-in-one-test",
        action="store_true",
        help=(
            "Single /rubix/v1/tx call combining RBT + every FT batch + every NFT + every SC. "
            "Auto-sets nft/sc/ft counts + executes."
        ),
    )
    p.add_argument(
        "--all-in-one-rounds",
        type=int,
        default=3,
        metavar="N",
        help="Number of all-in-one transaction rounds (default: 3).",
    )
    p.add_argument(
        "--all-in-one-rbt",
        type=float,
        default=1.0,
        metavar="AMT",
        help="RBT amount per all-in-one round (default: 1.0).",
    )
    p.add_argument(
        "--all-in-one-ft-amount",
        type=float,
        default=1.0,
        metavar="AMT",
        help="FTs moved per eligible batch per round (default: 1.0).",
    )
    # ------------------------------------------------------------------
    # Intra-node two-DID test
    # ------------------------------------------------------------------
    p.add_argument(
        "--intra-node-test",
        action="store_true",
        help=(
            "Create a second DID on nodeA (did_a2) and exercise intra-node RBT + FT "
            "back-and-forth plus NFT/SC deploy+self-execute by did_a2."
        ),
    )
    p.add_argument(
        "--intra-node-rbt-rounds",
        type=int,
        default=3,
        metavar="N",
        help="Number of RBT A<->A2 round-trips (default: 3).",
    )
    p.add_argument(
        "--intra-node-rbt-amount",
        type=float,
        default=1.0,
        metavar="AMT",
        help="RBT amount transferred per intra-node direction (default: 1.0).",
    )
    p.add_argument(
        "--intra-node-rbt-fund",
        type=float,
        default=5.0,
        metavar="AMT",
        help="Initial RBT fund did_a -> did_a2 before the ping-pong (default: 5.0).",
    )
    p.add_argument(
        "--intra-node-ft-rounds",
        type=int,
        default=2,
        metavar="N",
        help="Number of FT A<->A2 round-trips (default: 2).",
    )
    p.add_argument(
        "--intra-node-ft-amount",
        type=int,
        default=1,
        metavar="N",
        help="FT count moved per intra-node direction (default: 1).",
    )
    p.add_argument(
        "--intra-node-ft-fund",
        type=int,
        default=2,
        metavar="N",
        help="Initial FT fund did_a -> did_a2 before the FT ping-pong (default: 2).",
    )
    # ------------------------------------------------------------------
    # Mega flag — run every test category in one pass
    # ------------------------------------------------------------------
    p.add_argument(
        "--run-all-tests",
        action="store_true",
        help=(
            "Run EVERY test category: RBT shuttle, NFT (deploy+self+cross), "
            "SC (deploy+self+cross), FT (mint+transfer), bundled, all-in-one, intra-node. "
            "Auto-sets the relevant counts/flags and keeps the RBT shuttle enabled."
        ),
    )
    p.add_argument(
        "--skip-ft",
        action="store_true",
        help=(
            "Disable every FT-dependent test path. Composes with --run-all-tests / "
            "--bundled-test / --all-in-one-test."
        ),
    )
    # ------------------------------------------------------------------
    # Fullnode flags
    # ------------------------------------------------------------------
    p.add_argument(
        "--fullnode-test",
        action="store_true",
        help=(
            "Start a 4th node with `-fullnode` (compose profile `fullnode`) and "
            "verify the localnet transaction -> PubSub -> fullnode -> validation "
            "-> persistence path: subscription, receipt, persisted fields, quorum "
            "signatures, pledged tokens, chain integrity, no duplicates, and "
            "restart recovery. Docker mode only."
        ),
    )
    p.add_argument(
        "--no-fullnode-restart",
        action="store_true",
        help=(
            "Skip the fullnode restart/idempotency check within --fullnode-test "
            "(saves ~1-2 min; the rest of the fullnode suite still runs)."
        ),
    )
    p.add_argument(
        "--negative-tests",
        action="store_true",
        help=(
            "Run the negative / failure-path suite (invalid ops asserted to be "
            "rejected for the right reason + leave state unchanged): zero/insufficient "
            "balance, decimal-precision, FT over-transfer, invalid inputs. "
            "Auto-enabled by --run-all-tests."
        ),
    )
    return p


def main() -> None:
    args = _build_arg_parser().parse_args()

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(name)s] %(levelname)s: %(message)s",
    )

    cfg = StressConfig.load(
        config_path=args.config,
        small_test=args.small_test,
        micro_test=args.micro_test,
    )

    if args.max_transactions is not None:
        cfg.max_transactions = args.max_transactions
        log.info("max_transactions overridden to %d", cfg.max_transactions)

    log.info("Loaded config:\n%s", cfg.summary())

    # --- Flag auto-set handlers (order matters; mirrors the suite contract) ---

    # --run-all-tests: enable everything. Runs first so later handlers layer on.
    if args.run_all_tests:
        log.info("=== RUN-ALL-TESTS MODE ENABLED ===")
        if args.nft_count == 0:
            args.nft_count = 2
        if args.sc_count == 0:
            args.sc_count = 2
        if args.ft_count == 0:
            args.ft_count = 2
        args.nft_self_execute = True
        args.nft_cross_execute = True
        args.sc_execute = True
        args.ft_transfer = True
        args.bundled_test = True
        args.all_in_one_test = True
        args.intra_node_test = True
        args.negative_tests = True
        args.nft_only = False
        args.sc_only = False
        args.ft_only = False

    # --full-token-test
    if args.full_token_test:
        log.info("=== FULL TOKEN TEST MODE ENABLED ===")
        args.nft_count = 2
        args.nft_self_execute = True
        args.nft_transfer = True
        args.nft_cross_execute = True
        args.sc_count = 2
        args.sc_execute = True
        if args.exec_rounds == 0:
            args.exec_rounds = 10
        args.nft_only = True

    # Validate *-only requirements
    if args.nft_only and args.nft_count == 0:
        log.error("--nft-only requires --nft-count > 0")
        sys.exit(1)
    if args.sc_only and args.sc_count == 0:
        log.error("--sc-only requires --sc-count > 0")
        sys.exit(1)
    if args.ft_only and args.ft_count == 0:
        log.error("--ft-only requires --ft-count > 0")
        sys.exit(1)

    # --all-in-one-test: auto-set NFT + SC + FT prerequisites
    if args.all_in_one_test:
        log.info("=== ALL-IN-ONE TX MODE ENABLED ===")
        if args.nft_count == 0:
            args.nft_count = 2
        if args.sc_count == 0:
            args.sc_count = 2
        if args.ft_count == 0:
            args.ft_count = 2
        args.sc_execute = True
        args.nft_self_execute = True
        args.nft_cross_execute = True
        # The all-in-one rounds bundle+move the FTs themselves; a prior standalone
        # transfer would leave owner state stale. Keep freshly-minted FT batches.
        args.ft_transfer = False
        args.nft_only = True

    # --bundled-test: auto-set NFT + SC prerequisites
    if args.bundled_test:
        log.info("=== BUNDLED TEST MODE ENABLED ===")
        if args.nft_count == 0:
            args.nft_count = 2
        if args.sc_count == 0:
            args.sc_count = 2
        args.sc_execute = True
        args.nft_self_execute = True
        args.nft_cross_execute = True
        args.nft_only = True

    # Re-open the RBT shuttle gate that bundled/all-in-one just closed when the
    # user asked for everything.
    if args.run_all_tests:
        args.nft_only = False
        args.sc_only = False
        args.ft_only = False

    # --skip-ft applied LAST so it overrides auto-settings above.
    if args.skip_ft:
        log.info("=== --skip-ft ENABLED: disabling all FT-dependent test paths ===")
        args.ft_count = 0
        args.ft_only = False
        args.ft_transfer = False
        args.ft_transfer_rounds = 0
        args.all_in_one_test = False
        args.intra_node_ft_rounds = 0

    # Build NFT phases
    nft_phases = None
    if args.nft_count > 0:
        nft_phases = [
            {"name": "nft_sequential", "concurrency": 1, "tx_count": args.nft_count}
        ]
        log.info(
            "NFT enabled: %d NFTs (%s)",
            args.nft_count, "NFT-only" if args.nft_only else "after RBT transfers",
        )

    # Build FT phases
    ft_phases = None
    if args.ft_count > 0:
        ft_phases = [
            {"name": "ft_sequential", "concurrency": 1, "tx_count": args.ft_count}
        ]
        log.info(
            "FT enabled: %d mint ops (%d FTs/op, %d RBT burned/op)",
            args.ft_count, args.ft_tokens_per_batch, args.ft_rbt_per_batch,
        )

    if args.sc_count > 0:
        log.info(
            "Smart contract enabled: %d contracts (%s)",
            args.sc_count, "with cross-node execution" if args.sc_execute else "deploy only",
        )

    runner = StressRunner(cfg)

    # Environment owns cluster lifecycle:
    #   --native    => stand up native Postgres + rubixgoplatform processes (no Docker)
    #   --no-docker => caller provides already-running nodes (external environment)
    #   default     => spin up a fresh compose stack and tear it down on the way out
    if args.native and args.no_docker:
        log.error("--native and --no-docker are mutually exclusive.")
        sys.exit(2)
    if args.fullnode_test and (args.native or args.no_docker):
        # The fullnode node is a compose service; the native cluster manager
        # runs a fixed 3-process cluster and --no-docker provides its own nodes.
        # Failing loudly beats silently running a fullnode suite with no fullnode.
        log.error(
            "--fullnode-test requires the Docker environment "
            "(it starts the `fullnode` compose profile); "
            "it cannot be combined with --native or --no-docker."
        )
        sys.exit(2)
    if args.native:
        from test.integration.env.native_env import NativeEnvironment
        env = NativeEnvironment()
    elif args.no_docker:
        env = None
    else:
        env = DockerEnvironment(with_fullnode=args.fullnode_test)

    if args.fullnode_test:
        log.info("=== FULLNODE TEST MODE ENABLED (compose profile: fullnode) ===")
        # Hand the suite docker exec / logs / restart access to the fullnode
        # container so it can assert subscription and restart behaviour, plus
        # exec access to the publishers so the gossipsub mesh can be confirmed
        # from the side that actually decides delivery.
        runner.fullnode_controller = env.fullnode_controller()
        runner.publisher_controllers = {
            service: env.controller(service, container)
            for service, container in (
                ("nodeA", "rubix-stress-nodeA"),
                ("nodeB", "rubix-stress-nodeB"),
                ("quorum", "rubix-stress-quorum"),
            )
        }

    try:
        if env is not None:
            env.setup()
        runner.run(
            skip_setup=False,
            skip_mint=False,
            nft_phases=nft_phases,
            nft_only=args.nft_only,
            nft_self_execute=args.nft_self_execute,
            nft_transfer=args.nft_transfer,
            nft_cross_execute=args.nft_cross_execute,
            sc_count=args.sc_count,
            sc_execute=args.sc_execute,
            sc_only=args.sc_only,
            exec_rounds=args.exec_rounds,
            bundled_test=args.bundled_test,
            bundled_rounds=args.bundled_rounds,
            bundled_rbt=args.bundled_rbt,
            decimal_transfers=args.decimal_transfers,
            ft_phases=ft_phases,
            ft_only=args.ft_only,
            ft_transfer=args.ft_transfer,
            ft_transfer_rounds=args.ft_transfer_rounds,
            ft_tokens_per_batch=args.ft_tokens_per_batch,
            ft_rbt_per_batch=args.ft_rbt_per_batch,
            all_in_one_test=args.all_in_one_test,
            all_in_one_rounds=args.all_in_one_rounds,
            all_in_one_rbt=args.all_in_one_rbt,
            all_in_one_ft_amount=args.all_in_one_ft_amount,
            intra_node_test=args.intra_node_test,
            intra_node_rbt_rounds=args.intra_node_rbt_rounds,
            intra_node_rbt_amount=args.intra_node_rbt_amount,
            intra_node_rbt_fund=args.intra_node_rbt_fund,
            intra_node_ft_rounds=args.intra_node_ft_rounds,
            intra_node_ft_amount=args.intra_node_ft_amount,
            intra_node_ft_fund=args.intra_node_ft_fund,
            run_all_tests=args.run_all_tests,
            negative_tests=args.negative_tests,
            fullnode_test=args.fullnode_test,
            fullnode_restart_test=not args.no_fullnode_restart,
        )
    except Exception as exc:
        log.error("Integration run failed: %s", exc, exc_info=True)
        sys.exit(1)
    finally:
        if env is not None:
            env.teardown()

    # Map a failed verification check to a non-zero exit so CI fails the job.
    # The suite logs failures but does not raise; verification_failed carries
    # the count from the last verification summary.
    if runner.verification_failed > 0:
        log.error(
            "FAILED: %d verification check(s) did not pass — see verification.json",
            runner.verification_failed,
        )
        sys.exit(1)

    log.info("Integration run completed: all verification checks passed.")


if __name__ == "__main__":
    main()
