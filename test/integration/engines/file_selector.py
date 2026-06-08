"""
file_selector.py — Random file selection for NFT stress testing.

Provides utilities to select random files from the project directory,
ensuring uniqueness across parallel/concurrent NFT creation operations.
"""

import glob
import logging
import os
import random
from typing import List, Tuple

log = logging.getLogger(__name__)

# File size limit for NFT stress testing (5MB)
MAX_FILE_SIZE = 5 * 1024 * 1024

# Directories to exclude from file selection
EXCLUDE_DIRS = {
    ".git",
    "__pycache__",
    "node_modules",
    ".venv",
    "venv",
    "data",
    ".planning",
}


class FileSelector:
    """Select random files from project directory for NFT testing."""

    def __init__(self, base_dir: str = ".", max_size: int = MAX_FILE_SIZE) -> None:
        """Initialize file selector.

        Args:
            base_dir: Base directory to search for files (default: current directory)
            max_size: Maximum file size in bytes (default: 5MB)
        """
        self.base_dir = os.path.abspath(base_dir)
        self.max_size = max_size
        self._file_cache: List[str] = []
        self._cache_populated = False

    def _populate_cache(self) -> None:
        """Scan directory and cache suitable files."""
        if self._cache_populated:
            return

        log.info("Scanning %s for suitable NFT test files...", self.base_dir)

        all_files = []
        for root, dirs, files in os.walk(self.base_dir):
            # Skip excluded directories
            dirs[:] = [d for d in dirs if d not in EXCLUDE_DIRS]

            for file in files:
                # Skip hidden files and common non-suitable files
                if file.startswith(".") or file.endswith((".pyc", ".pyo", ".db")):
                    continue

                filepath = os.path.join(root, file)

                # Check file size
                try:
                    size = os.path.getsize(filepath)
                    if 0 < size <= self.max_size:
                        all_files.append(filepath)
                except OSError:
                    continue

        self._file_cache = all_files
        self._cache_populated = True
        log.info("Found %d suitable files for NFT testing", len(self._file_cache))

    def select_random_files(self, count: int = 2) -> Tuple[str, ...]:
        """Select random files from the project directory.

        Args:
            count: Number of files to select (default: 2 for metadata + artifact)

        Returns:
            Tuple of file paths

        Raises:
            RuntimeError: If not enough suitable files are available
        """
        self._populate_cache()

        if len(self._file_cache) < count:
            raise RuntimeError(
                f"Not enough suitable files for NFT testing. "
                f"Found {len(self._file_cache)}, need {count}. "
                f"Try increasing max_size or adding more files to the project."
            )

        # Random selection ensures uniqueness in parallel tests
        selected = random.sample(self._file_cache, count)
        log.debug("Selected files: %s", [os.path.basename(f) for f in selected])
        return tuple(selected)

    def select_metadata_and_artifact(self) -> Tuple[str, str]:
        """Select two random files for NFT metadata and artifact.

        Returns:
            (metadata_path, artifact_path)
        """
        metadata_path, artifact_path = self.select_random_files(count=2)
        return metadata_path, artifact_path


# Global instance for convenience
_default_selector: FileSelector = None


def get_default_selector() -> FileSelector:
    """Get or create the default FileSelector instance."""
    global _default_selector
    if _default_selector is None:
        _default_selector = FileSelector()
    return _default_selector


def select_nft_files() -> Tuple[str, str]:
    """Convenience function to select metadata and artifact files.

    Returns:
        (metadata_path, artifact_path)
    """
    return get_default_selector().select_metadata_and_artifact()


def select_smart_contract_files() -> Tuple[str, str]:
    """Select random .wasm and .rs files for smart contract testing.

    Returns:
        (wasm_path, source_path)

    Raises:
        RuntimeError: If no suitable .wasm or .rs files are found
    """
    selector = get_default_selector()
    selector._populate_cache()

    # Filter for .wasm files
    wasm_files = [f for f in selector._file_cache if f.endswith(".wasm")]
    # Filter for .rs files
    rs_files = [f for f in selector._file_cache if f.endswith(".rs")]

    if not wasm_files:
        raise RuntimeError(
            "No .wasm files found for smart contract testing. "
            "Please add .wasm binary files to the project."
        )

    if not rs_files:
        raise RuntimeError(
            "No .rs files found for smart contract testing. "
            "Please add .rs source files to the project."
        )

    # Select random files
    wasm_path = random.choice(wasm_files)
    source_path = random.choice(rs_files)

    log.debug("Selected SC files: wasm=%s, source=%s",
              os.path.basename(wasm_path), os.path.basename(source_path))

    return wasm_path, source_path
