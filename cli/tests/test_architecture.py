"""Architectural guardrails for twdll step modules."""

from __future__ import annotations

from pathlib import Path

_CLI_DIR = Path(__file__).resolve().parent.parent
_SRC_DIR = _CLI_DIR / "src" / "twdll"
_TESTS_DIR = _CLI_DIR / "tests"
_EXECUTION_STEP_MODULES = ("analyze.py", "extract.py")


def test_no_type_ignore_comments_in_src_or_tests() -> None:
    """Source and test modules must not bypass typechecking with ignore comments."""
    for module_path in sorted([*_SRC_DIR.rglob("*.py"), *_TESTS_DIR.rglob("*.py")]):
        if module_path.name == "test_architecture.py":
            continue
        source = module_path.read_text(encoding="utf-8")
        assert "# type: ignore" not in source, f"{module_path.name} contains '# type: ignore'"
        assert "# pyright: ignore" not in source, f"{module_path.name} contains '# pyright: ignore'"


def test_execution_steps_use_resolve_toolchain() -> None:
    """Execution steps must call `resolve_toolchain(ctx)`, never `ctx.config.toolchain`."""
    ghidra_files = sorted((_SRC_DIR / "_ghidra").rglob("*.py"))
    assert ghidra_files, "Expected non-empty twdll._ghidra package"
    for module_path in [*(_SRC_DIR / name for name in _EXECUTION_STEP_MODULES), *ghidra_files]:
        if not module_path.is_file():
            continue
        source = module_path.read_text(encoding="utf-8")
        assert ".config.toolchain" not in source, (
            f"{module_path.name} must use resolve_toolchain(ctx) instead of .config.toolchain"
        )
