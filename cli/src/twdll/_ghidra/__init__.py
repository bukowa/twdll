"""Internal PyGhidra lifecycle and DWARF injection passes."""

from __future__ import annotations

from twdll._ghidra.dwarf_layouts import (
    DwarfLayoutInputs,
    pass2_vtables_and_structs,
)
from twdll._ghidra.dwarf_locals import (
    DwarfLocalInputs,
    pass4_locals_and_inlines,
)
from twdll._ghidra.dwarf_signatures import (
    DwarfSignatureInputs,
    pass3_signatures,
)
from twdll._ghidra.dwarf_types import (
    pass1_type_shells,
)
from twdll._ghidra.lifecycle import (
    analyze_and_save_project,
    build_injection_report,
    ghidra_temp_dir,
    jarray,
    jclass,
    make_task_monitor,
    remove_ghidra_project,
    require_ghidra_toolchain,
    start_ghidra,
)

__all__ = [
    "DwarfLayoutInputs",
    "DwarfLocalInputs",
    "DwarfSignatureInputs",
    "analyze_and_save_project",
    "build_injection_report",
    "ghidra_temp_dir",
    "jarray",
    "jclass",
    "make_task_monitor",
    "pass1_type_shells",
    "pass2_vtables_and_structs",
    "pass3_signatures",
    "pass4_locals_and_inlines",
    "remove_ghidra_project",
    "require_ghidra_toolchain",
    "start_ghidra",
]
