"""Shared utilities for `tools/ghidra-audit`: normalization, tiering, and cache."""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import time
from collections.abc import Generator
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Literal

from twdll import _ghidra, database
from twdll.domain import (
    BinaryName,
    Decompiler,
    WorkspaceContext,
    compute_file_sha256,
    resolve_dwarf_exporter,
)

REPO_ROOT = Path(__file__).resolve().parents[2]
AUDIT_DIR = Path(__file__).resolve().parent
DEFAULT_RUNS_DIR = AUDIT_DIR / "runs"
DEFAULT_DWARF_DB = DEFAULT_RUNS_DIR / "attila_dwarf.sqlite"
DEFAULT_CACHE_DIR = DEFAULT_RUNS_DIR / "cache"
SNAPSHOT_STEM = "snapshot"

_STL_MIDDLEWARE_PREFIXES = (
    "std::",
    "__gnu_cxx::",
    "boost::",
    "tbb::",
    "Json::",
    "lua",
    "tinyxml",
    "zlib",
    "png_",
    "google::",
    "rapidxml::",
)
_ENGINE_PREFIXES = (
    "WARSCAPE::",
    "UTILITYLIB::",
    "CA::",
    "CA_STD::",
    "UTILITYDLL::",
)
_OPERATOR_TOKENS = (
    "operator<<=",
    "operator>>=",
    "operator<=",
    "operator>=",
    "operator<<",
    "operator>>",
    "operator->*",
    "operator->",
    "operator<",
    "operator>",
)


def normalize_template_skeleton(name: str) -> str:
    """Replace balanced `<...>` template args with `<T>`, preserving C++ operators."""
    out: list[str] = []
    i = 0
    n = len(name)
    while i < n:
        matched_op = next(
            (op for op in _OPERATOR_TOKENS if name.startswith(op, i)), None
        )
        if matched_op is not None:
            out.append(matched_op)
            i += len(matched_op)
            continue
        if name[i] == "<":
            depth = 1
            j = i + 1
            while j < n and depth > 0:
                if name[j] == "<":
                    depth += 1
                elif name[j] == ">":
                    depth -= 1
                j += 1
            if depth == 0:
                out.append("<T>")
                i = j
                continue
        out.append(name[i])
        i += 1
    return "".join(out)


def classify_namespace_tier(
    qualified_name: str,
) -> Literal["core", "engine", "stl_middleware"]:
    """Classify a qualified C++ symbol into `core`, `engine`, or `stl_middleware`."""
    qn = qualified_name.strip()
    if any(qn.startswith(prefix) for prefix in _STL_MIDDLEWARE_PREFIXES):
        return "stl_middleware"
    if any(qn.startswith(prefix) for prefix in _ENGINE_PREFIXES):
        return "engine"
    return "core"


def compute_cache_key(repo_root: Path, dwarf_db_path: Path) -> str:
    """Compute a SHA-256 key over analyzed `.meta.json`, DWARF DB, and CLI code."""
    meta_path = repo_root / ".twdll" / "analyze" / "ghidra_attila_linux_64.meta.json"
    inputs: list[tuple[str, Path]] = [
        ("analyze_meta", meta_path),
        ("dwarf_db", dwarf_db_path),
        ("domain", repo_root / "cli" / "src" / "twdll" / "domain.py"),
        ("database", repo_root / "cli" / "src" / "twdll" / "database.py"),
    ]
    ghidra_dir = repo_root / "cli" / "src" / "twdll" / "_ghidra"
    for py_file in sorted(ghidra_dir.glob("*.py")):
        inputs.append((f"_ghidra/{py_file.name}", py_file))

    h = hashlib.sha256()
    for label, path in inputs:
        digest = compute_file_sha256(path) or "missing"
        h.update(f"{label}:{digest}\n".encode())
    return h.hexdigest()


def rebuild_local_dwarf_db(ctx: WorkspaceContext, target_path: Path) -> Path:
    """Rebuild `target_path` from `twdll-dwarf` via staging + WAL checkpoint."""
    staging_path = target_path.with_suffix(".sqlite.staging")
    exporter = resolve_dwarf_exporter(ctx)
    elf_path = ctx.paths.staged_binary_path(BinaryName.ATTILA_LINUX_64)
    if not elf_path.is_file():
        raise FileNotFoundError(f"Staged ELF binary not found: {elf_path}")
    elf_sha = compute_file_sha256(elf_path)
    if elf_sha is None:
        raise FileNotFoundError(f"Failed computing SHA-256 for {elf_path}")

    database.init_extract_staging_db(staging_path)
    subprocess.run([str(exporter), str(elf_path), str(staging_path)], check=True)
    database.finalize_extract_staging_db(
        staging_path,
        target_path,
        binary_sha256=elf_sha,
        image_base=0,
    )
    return target_path


def _read_snapshot_meta(cache_dir: Path) -> dict[str, Any] | None:
    meta_path = cache_dir / "snapshot_meta.json"
    gpr_path = cache_dir / f"{SNAPSHOT_STEM}.gpr"
    rep_dir = cache_dir / f"{SNAPSHOT_STEM}.rep"
    if not (meta_path.is_file() and gpr_path.is_file() and rep_dir.is_dir()):
        return None
    try:
        data = json.loads(meta_path.read_text(encoding="utf-8"))
        return data if isinstance(data, dict) else None
    except (OSError, ValueError):
        return None


def _run_passes_sequentially(program: Any, db_path: Path) -> dict[str, Any]:
    t0 = time.monotonic()
    types, enums = database.load_dwarf_type_shells(db_path)
    type_map, type_rows, p1 = _ghidra.pass1_type_shells(program, types, enums)
    del types, enums
    print(
        f"[{time.monotonic() - t0:6.2f}s] Pass 1 complete ({len(type_map):,} types)",
        flush=True,
    )

    vtables, members, inheritances = database.load_dwarf_struct_layouts(db_path)
    vt_funcs, vt_params = database.load_dwarf_vtable_function_signatures(db_path)
    globals_ = database.load_dwarf_globals(db_path)
    p2 = _ghidra.pass2_vtables_and_structs(
        program,
        _ghidra.DwarfLayoutInputs(
            type_map=type_map,
            type_rows=type_rows,
            members=members,
            inheritances=inheritances,
            vtables=vtables,
            vtable_funcs=vt_funcs,
            vtable_params=vt_params,
            globals_=globals_,
        ),
    )
    del vtables, members, inheritances, vt_funcs, vt_params, globals_
    print(f"[{time.monotonic() - t0:6.2f}s] Pass 2 complete", flush=True)

    funcs, params = database.load_dwarf_function_signatures(db_path)
    p3 = _ghidra.pass3_signatures(
        program,
        _ghidra.DwarfSignatureInputs(
            type_map=type_map,
            type_rows=type_rows,
            functions=funcs,
            parameters=params,
        ),
    )
    del params
    print(f"[{time.monotonic() - t0:6.2f}s] Pass 3 complete", flush=True)

    variables, inlines = database.load_dwarf_locals_and_inlines(db_path)
    p4 = _ghidra.pass4_locals_and_inlines(
        program,
        _ghidra.DwarfLocalInputs(
            type_map=type_map,
            functions=funcs,
            variables=variables,
            inlines=inlines,
        ),
    )
    del variables, inlines, funcs, type_map, type_rows
    print(f"[{time.monotonic() - t0:6.2f}s] Pass 4 complete", flush=True)
    return _ghidra.build_injection_report(p1, p2, p3, p4).to_dict()


def ensure_cached_snapshot(
    ctx: WorkspaceContext,
    dwarf_db_path: Path,
    cache_dir: Path,
    *,
    skip_cache: bool = False,
) -> tuple[Path, dict[str, Any]]:
    """Return a valid single-slot DWARF-injected Ghidra snapshot `(cache_dir, meta)`."""
    cache_key = compute_cache_key(ctx.root, dwarf_db_path)
    existing = _read_snapshot_meta(cache_dir)
    if (
        not skip_cache
        and existing is not None
        and existing.get("cache_key") == cache_key
    ):
        return cache_dir, existing

    src_gpr = ctx.paths.ghidra_gpr_path(BinaryName.ATTILA_LINUX_64)
    src_rep = ctx.paths.ghidra_rep_dir(BinaryName.ATTILA_LINUX_64)
    if not src_gpr.is_file() or not src_rep.is_dir():
        raise FileNotFoundError(f"Base analyzed Ghidra project missing: {src_gpr}")

    cache_dir.mkdir(parents=True, exist_ok=True)
    staging = cache_dir / "_staging"
    if staging.exists():
        shutil.rmtree(staging)
    staging.mkdir(parents=True, exist_ok=True)

    shutil.copy2(src_gpr, staging / f"{SNAPSHOT_STEM}.gpr")
    shutil.copytree(src_rep, staging / f"{SNAPSHOT_STEM}.rep")

    _ghidra.start_ghidra(ctx)
    import pyghidra

    with (
        pyghidra.open_project(staging, SNAPSHOT_STEM, create=False) as project,
        pyghidra.program_context(
            project, f"/{BinaryName.ATTILA_LINUX_64.value}"
        ) as program,
    ):
        pass_stats = _run_passes_sequentially(program, dwarf_db_path)
        program.save("twdll-audit cached DWARF injection", None)

    meta = {
        "cache_key": cache_key,
        "decompiler": Decompiler.GHIDRA.value,
        "dwarf_db": str(dwarf_db_path),
        "pass_stats": pass_stats,
    }
    meta_tmp = staging / "snapshot_meta.json"
    meta_tmp.write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")

    target_gpr = cache_dir / f"{SNAPSHOT_STEM}.gpr"
    target_rep = cache_dir / f"{SNAPSHOT_STEM}.rep"
    target_meta = cache_dir / "snapshot_meta.json"
    if target_rep.exists():
        shutil.rmtree(target_rep)
    target_gpr.unlink(missing_ok=True)
    target_meta.unlink(missing_ok=True)

    os.replace(staging / f"{SNAPSHOT_STEM}.gpr", target_gpr)
    os.replace(staging / f"{SNAPSHOT_STEM}.rep", target_rep)
    os.replace(meta_tmp, target_meta)
    shutil.rmtree(staging, ignore_errors=True)
    return cache_dir, meta


@contextmanager
def open_cached_program_readonly(
    cache_dir: Path, ctx: WorkspaceContext | None = None
) -> Generator[Any]:
    """Open the cached snapshot strictly read-only (`getImmutableDomainObject`)."""
    import pyghidra

    if not pyghidra.started():
        _ghidra.start_ghidra(ctx or WorkspaceContext.from_root(REPO_ROOT))

    with pyghidra.open_project(cache_dir, SNAPSHOT_STEM, create=False) as project:
        df = project.getProjectData().getFile(f"/{BinaryName.ATTILA_LINUX_64.value}")
        if df is None:
            raise FileNotFoundError(
                f"Program /{BinaryName.ATTILA_LINUX_64.value} not found in {cache_dir}"
            )
        consumer = _ghidra.jclass("java.lang.Object")()
        domain_file_cls = _ghidra.jclass("ghidra.framework.model.DomainFile")
        monitor = _ghidra.jclass("ghidra.util.task.TaskMonitor").DUMMY
        if hasattr(df, "getImmutableDomainObject"):
            program = df.getImmutableDomainObject(
                consumer, domain_file_cls.DEFAULT_VERSION, monitor
            )
        else:
            program = df.getReadOnlyDomainObject(
                consumer, domain_file_cls.DEFAULT_VERSION, monitor
            )
        try:
            yield program
        finally:
            program.release(consumer)
