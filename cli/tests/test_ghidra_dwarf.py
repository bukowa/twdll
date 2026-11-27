"""Offline unit tests for `twdll._ghidra` DWARF Passes 1-4."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass, field
from typing import Any

import pytest

from twdll._ghidra import (
    DwarfLayoutInputs,
    DwarfLocalInputs,
    DwarfSignatureInputs,
    build_injection_report,
    lifecycle,
    pass1_type_shells,
    pass2_vtables_and_structs,
    pass3_signatures,
    pass4_locals_and_inlines,
)
from twdll._ghidra.dwarf_types import unwrap_type_row, unwrap_type_size
from twdll.domain import (
    DwarfEnumeratorRow,
    DwarfFunctionRow,
    DwarfGlobalRow,
    DwarfInlineRow,
    DwarfParameterRow,
    DwarfPassName,
    DwarfPassStats,
    DwarfTypeRow,
    DwarfVariableRow,
)


class _FakeAddr:
    def __init__(self, value: int) -> None:
        self.value = value

    def add(self, delta: int) -> _FakeAddr:
        return _FakeAddr(self.value + delta)

    def __eq__(self, other: object) -> bool:
        return isinstance(other, _FakeAddr) and self.value == other.value

    def __hash__(self) -> int:
        return hash(self.value)

    def __repr__(self) -> str:
        return f"0x{self.value:x}"


class _FakeAddrSpace:
    def getAddress(self, value: int) -> _FakeAddr:
        return _FakeAddr(value)


class _FakeAddrFactory:
    def __init__(self) -> None:
        self._space = _FakeAddrSpace()

    def getDefaultAddressSpace(self) -> _FakeAddrSpace:
        return self._space


class _FakeRegister:
    def __init__(self, name: str, size: int, offset: int) -> None:
        self._name = name
        self._size = size
        self._addr = _FakeAddr(0x1000_0000 + offset)

    def getAddress(self) -> _FakeAddr:
        return self._addr

    def getMinimumByteSize(self) -> int:
        return self._size

    def getName(self) -> str:
        return self._name


@dataclass
class _FakeVarnode:
    addr: _FakeAddr
    size: int


class _FakeVariableStorage:
    VOID_STORAGE: Any = "VOID_STORAGE"

    def __init__(self, _program: Any, *args: Any) -> None:
        self.args = args


class _FakeDataType:
    def __init__(self, name: str, length: int = 8) -> None:
        self.name = name
        self.length = length

    def getLength(self) -> int:
        return self.length

    def __eq__(self, other: object) -> bool:
        return isinstance(other, _FakeDataType) and self.name == other.name

    def __hash__(self) -> int:
        return hash(self.name)


class _FakeStructDataType(_FakeDataType):
    def __init__(self, cat: Any, name: str, length: int) -> None:
        super().__init__(f"{cat}/{name}", max(length, 1))
        self.fields: dict[int, tuple[Any, int, str]] = {}
        self.bitfields: list[tuple[int, int, int, Any, int, str]] = []

    def replaceAtOffset(self, off: int, dt: Any, sz: int, name: str, _comment: Any) -> None:
        self.fields[off] = (dt, sz, name)

    def insertBitFieldAt(self, *args: Any) -> None:
        off, byte_width, bo, base_dt, bit_sz, name, _comment = args
        self.bitfields.append((off, byte_width, bo, base_dt, bit_sz, name))


class _FakeUnionDataType(_FakeDataType):
    def __init__(self, cat: Any, name: str) -> None:
        super().__init__(f"{cat}/{name}", 0)
        self.arms: list[tuple[Any, int, str]] = []
        self.fail_raw = False

    def add(self, dt: Any, length: int, name: str, _comment: Any) -> None:
        if name == "_raw" and self.fail_raw:
            raise RuntimeError("raw failed")
        self.arms.append((dt, length, name))
        self.length = max(self.length, length)

    def addBitField(self, dt: Any, bits: int, name: str, _comment: Any) -> None:
        self.arms.append((dt, bits, name))
        self.length = max(self.length, (bits + 7) // 8)


class _FakeEnumDataType(_FakeDataType):
    def __init__(self, cat: Any, name: str, length: int) -> None:
        super().__init__(f"{cat}/{name}", length)
        self.entries: list[tuple[str, int]] = []

    def add(self, name: str, value: int) -> None:
        if name == "BAD_ENUM":
            raise ValueError("duplicate enum value")
        self.entries.append((name, value))


class _FakeFuncDefDataType(_FakeDataType):
    def __init__(self, cat: Any, name: str) -> None:
        super().__init__(f"{cat}/{name}", 8)
        self.ret_dt: Any = None
        self.args: list[Any] = []

    def setReturnType(self, dt: Any) -> None:
        if getattr(dt, "name", "") == "BAD_RET":
            raise ValueError("bad return")
        self.ret_dt = dt

    def setArguments(self, args: list[Any]) -> None:
        self.args = list(args)


class _FakeDataTypeManager:
    def __init__(self) -> None:
        self.added: dict[str, Any] = {}

    def addDataType(self, dt: Any, _handler: Any) -> Any:
        self.added[dt.name] = dt
        return dt

    def getPointer(self, dt: Any, size: int = 8) -> _FakeDataType:
        return _FakeDataType(f"{dt.name}*", size)


@dataclass
class _FakeMemoryBlock:
    start: int
    end: int
    initialized: bool = True
    executable: bool = False

    def isInitialized(self) -> bool:
        return self.initialized

    def isExecute(self) -> bool:
        return self.executable


class _FakeMemory:
    def __init__(self, blocks: Sequence[_FakeMemoryBlock]) -> None:
        self._blocks = list(blocks)

    def getBlock(self, addr: _FakeAddr) -> _FakeMemoryBlock | None:
        for blk in self._blocks:
            if blk.start <= addr.value < blk.end:
                return blk
        return None


class _BoolIter:
    def __init__(self, has_next: bool) -> None:
        self._has_next = has_next

    def hasNext(self) -> bool:
        return self._has_next


class _FlowType:
    def isTerminal(self) -> bool:
        return True


class _TerminalInsn:
    def getFlowType(self) -> _FlowType:
        return _FlowType()


class _CodeUnit:
    def __init__(self, listing: _FakeListing, addr: _FakeAddr) -> None:
        self._listing = listing
        self._addr = addr

    def setComment(self, _ctype: Any, text: str) -> None:
        self._listing.comments[self._addr.value] = text


class _FakeListing:
    def __init__(self, insn_addrs: set[int] | None = None) -> None:
        self.cleared: list[tuple[int, int]] = []
        self.created_data: list[tuple[int, Any]] = []
        self.comments: dict[int, str] = {}
        self.insn_addrs = insn_addrs or set()
        self.terminal_addrs: set[int] = set()

    def clearCodeUnits(self, start: _FakeAddr, end: _FakeAddr, _clear_ctx: bool) -> None:
        self.cleared.append((start.value, end.value))

    def createData(self, addr: _FakeAddr, dt: Any) -> None:
        self.created_data.append((addr.value, dt))

    def getInstructions(self, addr_set: Any, _forward: bool) -> _BoolIter:
        has_insn = any(addr_set.start.value <= a <= addr_set.end.value for a in self.insn_addrs)
        return _BoolIter(has_insn)

    def getInstructionContaining(self, addr: _FakeAddr) -> _TerminalInsn | None:
        return _TerminalInsn() if addr.value in self.terminal_addrs else None

    def getCodeUnitAt(self, addr: _FakeAddr) -> _CodeUnit:
        return _CodeUnit(self, addr)


class _NsSymbol:
    def __init__(self, stype: str) -> None:
        self._stype = stype

    def getSymbolType(self) -> str:
        return self._stype


class _PrimarySymbol:
    def __init__(self, src: str) -> None:
        self._src = src

    def getSource(self) -> str:
        return self._src


class _FakeNamespace:
    def __init__(
        self, name: str, parent: _FakeNamespace | None = None, *, is_lib: bool = False
    ) -> None:
        self._name = name
        self._parent = parent
        self._is_lib = is_lib

    def isGlobal(self) -> bool:
        return self._parent is None

    def getName(self) -> str:
        return self._name

    def getParentNamespace(self) -> _FakeNamespace | None:
        return self._parent

    def getSymbol(self) -> _NsSymbol:
        return _NsSymbol("LIBRARY" if self._is_lib else "NAMESPACE")


class _FakeSymbolTable:
    def __init__(self) -> None:
        self.primary_symbols: dict[int, Any] = {}
        self.created_labels: list[tuple[int, str, str, str]] = []
        self.namespaces: dict[tuple[str, str], _FakeNamespace] = {}

    def getPrimarySymbol(self, addr: _FakeAddr) -> Any:
        return self.primary_symbols.get(addr.value)

    def getOrCreateNameSpace(self, parent: _FakeNamespace, name: str, _src: Any) -> _FakeNamespace:
        key = (parent.getName(), name)
        if key not in self.namespaces:
            self.namespaces[key] = _FakeNamespace(name, parent)
        return self.namespaces[key]

    def createLabel(self, addr: _FakeAddr, name: str, ns: _FakeNamespace, src: Any) -> None:
        self.created_labels.append((addr.value, name, ns.getName(), str(src)))

    def getSymbols(self, _addr: _FakeAddr) -> list[Any]:
        return []


class _ExtLoc:
    def __init__(self, orig: str) -> None:
        self._orig = orig

    def getOriginalImportedName(self) -> str:
        return self._orig


class _Body:
    def __init__(self, max_addr: int) -> None:
        self._max_addr = max_addr

    def getMaxAddress(self) -> _FakeAddr:
        return _FakeAddr(self._max_addr)


@dataclass
class _FakeFunction:
    addr: int
    name: str
    ns: _FakeNamespace
    external: bool = False
    thunk_target: _FakeFunction | None = None
    orig_imported_name: str | None = None
    noreturn: bool = False
    max_addr: int = 0
    updates: list[tuple[Any, list[Any], Any]] = field(default_factory=list)
    locals_added: list[tuple[Any, Any]] = field(default_factory=list)
    params: list[Any] = field(default_factory=list)

    def getName(self, include_ns: bool = False) -> str:
        if not include_ns:
            return self.name
        parts: list[str] = []
        cur: _FakeNamespace | None = self.ns
        while cur is not None and not cur.isGlobal():
            parts.append(cur.getName())
            cur = cur.getParentNamespace()
        parts.reverse()
        return "::".join([*parts, self.name]) if parts else self.name

    def getParentNamespace(self) -> _FakeNamespace:
        return self.ns

    def getExternalLocation(self) -> _ExtLoc | None:
        if self.orig_imported_name is None:
            return None
        return _ExtLoc(self.orig_imported_name)

    def hasNoReturn(self) -> bool:
        return self.noreturn

    def setNoReturn(self, val: bool) -> None:
        self.noreturn = val

    def isExternal(self) -> bool:
        return self.external

    def isThunk(self) -> bool:
        return self.thunk_target is not None

    def getThunkedFunction(self, _resolve: bool = True) -> _FakeFunction | None:
        return self.thunk_target

    def setThunkedFunction(self, target: Any) -> None:
        self.thunk_target = target

    def getBody(self) -> _Body:
        return _Body(self.max_addr)

    def getParameters(self) -> list[Any]:
        return list(self.params)

    def updateFunction(
        self,
        _cc: Any,
        ret_param: Any,
        j_params: list[Any],
        update_type: Any,
        _force: bool,
        _src: Any,
    ) -> None:
        self.updates.append((ret_param, list(j_params), update_type))

    def addLocalVariable(self, lv: Any, src: Any) -> None:
        self.locals_added.append((lv, src))


class _FakeFunctionManager:
    def __init__(self, funcs: Sequence[_FakeFunction]) -> None:
        self._funcs = list(funcs)

    def getExternalFunctions(self) -> list[_FakeFunction]:
        return [f for f in self._funcs if f.external]

    def getFunctions(self, _forward: bool) -> list[_FakeFunction]:
        return [f for f in self._funcs if not f.external]

    def getFunctionAt(self, addr: _FakeAddr) -> _FakeFunction | None:
        return next((f for f in self._funcs if not f.external and f.addr == addr.value), None)


class _FakeProgram:
    def __init__(
        self,
        *,
        blocks: Sequence[_FakeMemoryBlock] = (),
        funcs: Sequence[_FakeFunction] = (),
        insn_addrs: set[int] | None = None,
    ) -> None:
        self.dtm = _FakeDataTypeManager()
        self.listing = _FakeListing(insn_addrs=insn_addrs)
        self.mem = _FakeMemory(blocks)
        self.sym_table = _FakeSymbolTable()
        self.fm = _FakeFunctionManager(funcs)
        self.addr_factory = _FakeAddrFactory()
        self.global_ns = _FakeNamespace("global", None)
        self.regs: dict[str, _FakeRegister] = {
            name: _FakeRegister(name, 8, idx * 8)
            for idx, name in enumerate(
                ("RAX", "RDX", "RDI", "RSI", "RCX", "R8", "R9", "XMM0", "XMM1", "XMM2")
            )
        }

    def startTransaction(self, _name: str) -> int:
        return 1

    def endTransaction(self, _tx: int, _commit: bool) -> None:
        pass

    def clearUndo(self) -> None:
        pass

    def getDataTypeManager(self) -> _FakeDataTypeManager:
        return self.dtm

    def getListing(self) -> _FakeListing:
        return self.listing

    def getMemory(self) -> _FakeMemory:
        return self.mem

    def getSymbolTable(self) -> _FakeSymbolTable:
        return self.sym_table

    def getFunctionManager(self) -> _FakeFunctionManager:
        return self.fm

    def getAddressFactory(self) -> _FakeAddrFactory:
        return self.addr_factory

    def getGlobalNamespace(self) -> _FakeNamespace:
        return self.global_ns

    def getRegister(self, name: str) -> _FakeRegister | None:
        return self.regs.get(name)


class _ArrayDT(_FakeDataType):
    def __init__(self, dt: Any, count: int, elem_len: int) -> None:
        super().__init__(f"{dt.name}[{count}]", count * elem_len)


class _TypedefDT(_FakeDataType):
    def __init__(self, cat: Any, name: str, target: Any) -> None:
        super().__init__(f"{cat}/{name}", target.getLength())


class _BitFieldDT:
    @staticmethod
    def isValidBaseDataType(_dt: Any) -> bool:
        return True


class _ParamDef:
    def __init__(self, name: str, dt: Any, _comment: Any) -> None:
        self.name = name
        self.dt = dt


class _ParamImpl:
    def __init__(self, name: str, dt: Any, storage: Any, _program: Any) -> None:
        self.name = name
        self.dt = dt
        self.storage = storage


class _RetParamImpl:
    def __init__(self, dt: Any, storage: Any, _program: Any) -> None:
        self.dt = dt
        self.storage = storage


class _LocalVarImpl:
    def __init__(self, name: str, offset: int, dt: Any, storage: Any, _program: Any) -> None:
        self.name = name
        self.offset = offset
        self.dt = dt
        self.storage = storage


class _AddrSet:
    def __init__(self, start: _FakeAddr, end: _FakeAddr) -> None:
        self.start = start
        self.end = end


class _FuncCls:
    DEFAULT_CALLING_CONVENTION_STRING = "default"
    FunctionUpdateType = type("_FUT", (), {"CUSTOM_STORAGE": "CUSTOM_STORAGE"})()


class _SourceType:
    DEFAULT = "DEFAULT"
    IMPORTED = "IMPORTED"


class _SymbolType:
    LIBRARY = "LIBRARY"


class _CommentType:
    PRE = "PRE"


class _ConflictHandler:
    KEEP_HANDLER = "KEEP"
    REPLACE_HANDLER = "REPLACE"


class _DataTypeHolder:
    def __init__(self, dt: _FakeDataType) -> None:
        self.dataType = dt


class _FakeArrayList(list[Any]):
    def add(self, item: Any) -> bool:
        self.append(item)
        return True


def _fake_category_path(path: str) -> str:
    return path


def _fake_jarray(_cls: str, items: Sequence[Any]) -> list[Any]:
    return list(items)


def _install_fake_ghidra_classes(monkeypatch: pytest.MonkeyPatch) -> None:
    void_dt = _FakeDataType("void", 0)
    ll_dt = _FakeDataType("long long", 8)
    int_dt = _FakeDataType("int", 4)
    flt_dt = _FakeDataType("float", 4)
    dbl_dt = _FakeDataType("double", 8)
    undef1_dt = _FakeDataType("undefined1", 1)

    mapping: dict[str, Any] = {
        "ghidra.program.model.data.VoidDataType": _DataTypeHolder(void_dt),
        "ghidra.program.model.data.LongLongDataType": _DataTypeHolder(ll_dt),
        "ghidra.program.model.data.IntegerDataType": _DataTypeHolder(int_dt),
        "ghidra.program.model.data.FloatDataType": _DataTypeHolder(flt_dt),
        "ghidra.program.model.data.DoubleDataType": _DataTypeHolder(dbl_dt),
        "ghidra.program.model.data.Undefined1DataType": _DataTypeHolder(undef1_dt),
        "ghidra.program.model.data.UnsignedLongLongDataType": _DataTypeHolder(ll_dt),
        "ghidra.program.model.data.UnsignedIntegerDataType": _DataTypeHolder(int_dt),
        "ghidra.program.model.data.StructureDataType": _FakeStructDataType,
        "ghidra.program.model.data.UnionDataType": _FakeUnionDataType,
        "ghidra.program.model.data.EnumDataType": _FakeEnumDataType,
        "ghidra.program.model.data.FunctionDefinitionDataType": _FakeFuncDefDataType,
        "ghidra.program.model.data.ArrayDataType": _ArrayDT,
        "ghidra.program.model.data.TypedefDataType": _TypedefDT,
        "ghidra.program.model.data.BitFieldDataType": _BitFieldDT,
        "ghidra.program.model.data.ParameterDefinitionImpl": _ParamDef,
        "ghidra.program.model.data.DataTypeConflictHandler": _ConflictHandler,
        "ghidra.program.model.data.CategoryPath": _fake_category_path,
        "ghidra.program.model.pcode.Varnode": _FakeVarnode,
        "ghidra.program.model.listing.VariableStorage": _FakeVariableStorage,
        "ghidra.program.model.listing.ParameterImpl": _ParamImpl,
        "ghidra.program.model.listing.ReturnParameterImpl": _RetParamImpl,
        "ghidra.program.model.listing.LocalVariableImpl": _LocalVarImpl,
        "ghidra.program.model.listing.Function": _FuncCls,
        "ghidra.program.model.listing.CommentType": _CommentType,
        "ghidra.program.model.symbol.SourceType": _SourceType,
        "ghidra.program.model.symbol.SymbolType": _SymbolType,
        "ghidra.program.model.address.AddressSet": _AddrSet,
        "java.util.ArrayList": _FakeArrayList,
    }
    monkeypatch.setattr(lifecycle, "jclass", mapping.__getitem__)
    monkeypatch.setattr(lifecycle, "jarray", _fake_jarray)


def _trow(
    type_id: int,
    kind: str,
    qname: str,
    **kwargs: Any,
) -> DwarfTypeRow:
    return DwarfTypeRow(
        type_id=type_id,
        kind=kind,
        qualified_name=qname,
        byte_size=kwargs.get("byte_size"),
        target_type_id=kwargs.get("target_type_id"),
        array_count=None,
        containing_type_id=None,
        vtable_symbol=None,
        vtable_address=kwargs.get("vtable_address"),
        is_declaration=False,
        is_non_trivial_for_calls=kwargs.get("is_non_trivial", False),
        is_stl_stub=False,
        is_referenced=True,
        abi_class=kwargs.get("abi_class"),
    )


def test_pass1_ptr_to_member_and_telemetry(monkeypatch: pytest.MonkeyPatch) -> None:
    _install_fake_ghidra_classes(monkeypatch)
    prog = _FakeProgram()
    types = (
        _trow(1, "primitive", "void", byte_size=0),
        _trow(2, "primitive", "int", byte_size=4, abi_class="int"),
        _trow(3, "subroutine", "void()", byte_size=None, target_type_id=999),
        _trow(4, "typedef", "fn_t", target_type_id=3),
        _trow(5, "ptr_to_member", "pm_data", target_type_id=2),
        _trow(6, "ptr_to_member", "pm_func", target_type_id=4),
        _trow(7, "enum", "E", byte_size=4),
    )
    enums = (
        DwarfEnumeratorRow(7, 0, "OK", 1),
        DwarfEnumeratorRow(7, 1, "BAD_ENUM", 1),
    )
    type_map, type_rows, stats = pass1_type_shells(prog, types, enums)
    prog.dtm.added["BAD_RET"] = _FakeDataType("BAD_RET", 4)
    assert type_map[5].name == "long long"
    assert type_map[6].name == "/dwarf/_abi/__ptr_to_member_func"
    assert type_map[6].getLength() == 16
    assert unwrap_type_size(5, type_rows) == 8
    assert unwrap_type_size(6, type_rows) == 16
    assert unwrap_type_row(4, type_rows) == types[2]
    assert dict(stats.error_counts) == {"enum_add_err": 1}


def test_pass2_unions_and_bss_safe_globals(monkeypatch: pytest.MonkeyPatch) -> None:
    _install_fake_ghidra_classes(monkeypatch)
    prog = _FakeProgram(
        blocks=(
            _FakeMemoryBlock(0x500000, 0x501000, initialized=False, executable=False),
            _FakeMemoryBlock(0x400000, 0x401000, initialized=True, executable=True),
        ),
        insn_addrs={0x500100},
    )
    prog.sym_table.primary_symbols[0x500040] = _PrimarySymbol("IMPORTED")
    vt_struct = _FakeStructDataType("/dwarf", "UNIT_vtbl", 8)
    vt_struct.length = 8
    union_dt = _FakeUnionDataType("/dwarf", "U")
    union_dt.fail_raw = True
    type_map: dict[int, Any] = {
        1: _FakeDataType("int", 4),
        2: union_dt,
    }
    type_rows = {
        1: _trow(1, "primitive", "int", byte_size=4, abi_class="int"),
        2: _trow(2, "union", "U", byte_size=8),
    }
    globals_ = (
        DwarfGlobalRow(1, 1, 0x500010, None, "EMPIRE::s_bss", 1, 4, True),
        DwarfGlobalRow(2, 1, 0x500012, None, "EMPIRE::s_overlap", 1, 4, False),
        DwarfGlobalRow(3, 1, 0x500040, None, "EMPIRE::s_existing_label", 1, 4, False),
        DwarfGlobalRow(4, 1, 0x5000FE, None, "EMPIRE::s_insn_overlap", 1, 4, False),
        DwarfGlobalRow(5, 1, 0x400010, None, "EMPIRE::s_exec_skip", 1, 4, False),
    )
    inputs = DwarfLayoutInputs(
        type_map=type_map,
        type_rows=type_rows,
        members=(),
        inheritances=(),
        vtables=(),
        vtable_funcs={},
        vtable_params={},
        globals_=globals_,
    )
    stats = pass2_vtables_and_structs(prog, inputs)
    counts = dict(stats.counts)
    assert counts["globals_stamped"] == 2
    assert counts["globals_labeled"] == 1
    assert counts["globals_skipped_overlap"] == 1
    assert counts["globals_skipped_code"] == 1
    assert counts["globals_skipped_block"] == 1
    assert prog.sym_table.created_labels == [(0x500010, "s_bss", "EMPIRE", "IMPORTED")]


def test_pass3_abi_returns_varnodes_and_atomic_spill(monkeypatch: pytest.MonkeyPatch) -> None:
    _install_fake_ghidra_classes(monkeypatch)
    g_ns = _FakeNamespace("global", None)
    lib_ns = _FakeNamespace("libstdc++.so.6", g_ns, is_lib=True)
    std_ns = _FakeNamespace("std", lib_ns)
    funcs = (
        _FakeFunction(0, "__throw_bad_alloc", std_ns, external=True),
        _FakeFunction(0x401000, "__throw_bad_alloc_thunk", g_ns, thunk_target=None),
        _FakeFunction(0x402000, "panic", g_ns, noreturn=True, max_addr=0x402010),
        _FakeFunction(0x403000, "ret_13B_mixed", g_ns),
        _FakeFunction(0x404000, "atomic_spill_fn", g_ns),
    )
    funcs[1].thunk_target = funcs[0]
    prog = _FakeProgram(funcs=funcs)
    prog.listing.terminal_addrs.add(0x402010)

    type_rows = {
        1: _trow(1, "primitive", "void", byte_size=0),
        2: _trow(2, "primitive", "double", byte_size=8, abi_class="sse"),
        3: _trow(3, "struct", "Mixed13", byte_size=13, abi_class="sse,int"),
        4: _trow(4, "primitive", "int", byte_size=4, abi_class="int"),
    }
    type_map = {
        1: _FakeDataType("void", 0),
        2: _FakeDataType("double", 8),
        3: _FakeDataType("Mixed13", 13),
        4: _FakeDataType("int", 4),
    }
    fn_rows = (
        DwarfFunctionRow(
            1,
            1,
            0x403000,
            16,
            None,
            "ret_13B_mixed",
            None,
            3,
            None,
            0,
            False,
            False,
            False,
            False,
            False,
            None,
            None,
        ),
        DwarfFunctionRow(
            2,
            1,
            0x404000,
            16,
            None,
            "atomic_spill_fn",
            None,
            1,
            None,
            0,
            False,
            False,
            False,
            False,
            False,
            None,
            None,
        ),
    )
    # 6 integer scalar params exhaust RDI..R9; then Mixed13 (sse,int) MUST atomically spill to stack
    # without consuming XMM0, leaving XMM0 free for the trailing double param!
    param_rows = (
        *(
            DwarfParameterRow(2, i, f"i{i}", 4, False, False, "none", None, None, None)
            for i in range(6)
        ),
        DwarfParameterRow(2, 6, "m13", 3, False, False, "none", None, None, None),
        DwarfParameterRow(2, 7, "d_after", 2, False, False, "none", None, None, None),
    )
    stats = pass3_signatures(
        prog,
        DwarfSignatureInputs(
            type_map=type_map, type_rows=type_rows, functions=fn_rows, parameters=param_rows
        ),
    )
    assert not stats.error_counts, stats.sample_errors
    assert funcs[0].hasNoReturn() is True
    assert funcs[1].hasNoReturn() is True
    assert funcs[2].hasNoReturn() is False

    # Return of 13B sse,int -> MSB-first [Varnode(RAX, 5), Varnode(XMM0, 8)]
    ret_storage = funcs[3].updates[0][0].storage
    vnodes = ret_storage.args[0]
    assert [(v.addr, v.size) for v in vnodes] == [
        (prog.regs["RAX"].getAddress(), 5),
        (prog.regs["XMM0"].getAddress(), 8),
    ]

    # Atomic spill: m13 goes to stack (offset 8, size 13), d_after gets XMM0!
    spill_params = funcs[4].updates[0][1]
    assert spill_params[6].storage.args == (8, 13)
    assert spill_params[7].storage.args[0][0].addr == prog.regs["XMM0"].getAddress()
    assert dict(stats.counts)["param_2reg_atomic_spill"] == 1


def test_pass4_and_build_injection_report(monkeypatch: pytest.MonkeyPatch) -> None:
    _install_fake_ghidra_classes(monkeypatch)
    g_ns = _FakeNamespace("global", None)
    fn = _FakeFunction(0x405000, "local_fn", g_ns)
    prog = _FakeProgram(funcs=(fn,))
    type_map = {1: _FakeDataType("int", 4)}
    fn_rows = (
        DwarfFunctionRow(
            1,
            1,
            0x405000,
            16,
            None,
            "local_fn",
            None,
            None,
            None,
            0,
            False,
            False,
            False,
            False,
            False,
            None,
            None,
        ),
    )
    vars_ = (
        DwarfVariableRow(1, 1, "ok_stack", 1, 0, 0x405000, 0x405010, 0, "stack", None, -16, None),
        DwarfVariableRow(2, 1, "bad_stack", 1, 0, 0x405000, 0x405010, 0, "stack", None, 8, None),
        DwarfVariableRow(
            3, 1, "entry_reg", 1, 0, 0x405000, 0x405010, 0, "register", "RAX", None, None
        ),
        DwarfVariableRow(
            4, 1, "ok_reg", 1, 0, 0x405004, 0x405010, 4, "register", "RAX", None, None
        ),
    )
    inlines = (DwarfInlineRow(1, 1, None, "helper", 0x405004, 0x405008, "a.cpp", 12),)
    p4_stats = pass4_locals_and_inlines(
        prog,
        DwarfLocalInputs(type_map=type_map, functions=fn_rows, variables=vars_, inlines=inlines),
    )
    assert dict(p4_stats.counts) == {
        "inline_comments_set": 1,
        "local_reg_entry_skip": 1,
        "local_stack_oob": 1,
        "locals_reg_added": 1,
        "locals_stack_added": 1,
    }
    assert prog.listing.comments[0x405004] == "inline helper @ a.cpp:12"

    p1 = DwarfPassStats(DwarfPassName.PASS1_TYPES, error_counts=(("e1", 2),))
    p2 = DwarfPassStats(DwarfPassName.PASS2_LAYOUTS, error_counts=(("e2", 1),))
    p3 = DwarfPassStats(DwarfPassName.PASS3_SIGNATURES)
    report = build_injection_report(p1, p2, p3, p4_stats)
    assert report.total_errors == 3


def test_pass2_secondary_vtable_bfs_ownership(monkeypatch: pytest.MonkeyPatch) -> None:
    from twdll.domain import DwarfInheritanceRow, DwarfMemberRow, DwarfVtableEntryRow

    _install_fake_ghidra_classes(monkeypatch)
    prog = _FakeProgram()
    t_rows = {
        1: DwarfTypeRow(
            1,
            "class",
            "EMPIREBATTLE::SHIP",
            7144,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        2: DwarfTypeRow(
            2,
            "class",
            "EMPIREBATTLE::BUOYANT_OBJECT",
            312,
            None,
            None,
            None,
            "_ZTV14BUOYANT_OBJECT",
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        3: DwarfTypeRow(
            3,
            "class",
            "EMPIREBATTLE::TOGGLE_OBJECT",
            16,
            None,
            None,
            None,
            "_ZTV13TOGGLE_OBJECT",
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        4: DwarfTypeRow(
            4,
            "class",
            "EMPIREBATTLE::PROJECTILE",
            64,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        5: DwarfTypeRow(
            5,
            "class",
            "EMPIREUTILITY::SAFER_REPORTER<X>",
            32,
            None,
            None,
            None,
            "_ZTV14SAFER_REPORTER",
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        6: DwarfTypeRow(
            6,
            "class",
            "EMPIREUTILITY::SAFER_REPORTER_BASE",
            8,
            None,
            None,
            None,
            "_ZTV19SAFER_REPORTER_BASE",
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        7: DwarfTypeRow(
            7,
            "pointer",
            "void *",
            8,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "int",
        ),
    }
    ship_dt = _FakeStructDataType("EMPIREBATTLE", "SHIP", 7144)
    proj_dt = _FakeStructDataType("EMPIREBATTLE", "PROJECTILE", 64)
    t_map = {1: ship_dt, 4: proj_dt, 7: _FakeDataType("void *", 8)}
    inhs = (
        DwarfInheritanceRow(1, 1, 2, 0, None, False, False, False),
        DwarfInheritanceRow(2, 1, 3, 584, None, False, False, False),
        DwarfInheritanceRow(3, 4, 5, 16, None, False, False, False),
        DwarfInheritanceRow(4, 5, 6, 0, None, False, False, False),
    )
    mems = (
        DwarfMemberRow(1, 1, 0, "_vptr$TOGGLE_OBJECT", 7, 584, 8, None, None, True, False, False),
        DwarfMemberRow(
            2, 6, 0, "_vptr$SAFER_REPORTER_BASE", 7, 0, 8, None, None, True, False, False
        ),
    )
    vts = (
        DwarfVtableEntryRow(2, 0, "buoy", None, None, None, False),
        DwarfVtableEntryRow(3, 0, "enabled", None, None, None, False),
        DwarfVtableEntryRow(5, 0, "notify_listeners", None, None, None, False),
        DwarfVtableEntryRow(6, 0, "issue_postponed_messages", None, None, None, False),
    )
    stats = pass2_vtables_and_structs(
        prog,
        DwarfLayoutInputs(
            type_map=t_map,
            type_rows=t_rows,
            members=mems,
            inheritances=inhs,
            vtables=vts,
            vtable_funcs={},
            vtable_params={},
            globals_=(),
        ),
    )
    assert not stats.error_counts
    assert ship_dt.fields[584][0].name == "/dwarf/EMPIREBATTLE/TOGGLE_OBJECT_vtbl*"
    assert proj_dt.fields[16][0].name == "/dwarf/EMPIREUTILITY/SAFER_REPORTER<X>_vtbl*"


def test_pass3_adjusting_thunk_and_lua_noreturn(monkeypatch: pytest.MonkeyPatch) -> None:
    from twdll._ghidra.dwarf_signatures import STATIC_NORETURN_NAMES

    for lua_name in ("luaG_runerror", "luaL_error", "luaD_throw", "lua_error"):
        assert lua_name in STATIC_NORETURN_NAMES

    _install_fake_ghidra_classes(monkeypatch)
    g_ns = _FakeNamespace("global", None)
    thunk_fn = _FakeFunction(0x406000, "TWIN::WINDOW::on_mouse_up", g_ns)
    prog = _FakeProgram(funcs=(thunk_fn,))
    t_rows = {
        1: DwarfTypeRow(
            1,
            "class",
            "TWIN::WINDOW",
            64,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        2: DwarfTypeRow(
            2,
            "pointer",
            "TWIN::WINDOW *",
            8,
            1,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "int",
        ),
    }
    t_map = {
        1: _FakeStructDataType("TWIN", "WINDOW", 64),
        2: _FakeDataType("TWIN::WINDOW *", 8),
    }
    fn_row = DwarfFunctionRow(
        1,
        1,
        0x406000,
        32,
        "_ZThn24_N4TWIN6WINDOW11on_mouse_upEv",
        "subprog::unit1::0x1",
        None,
        None,
        None,
        0,
        False,
        False,
        False,
        True,
        False,
        None,
        None,
    )
    p_row = DwarfParameterRow(1, 0, "this", 2, True, True, "register", 5, "RDI", None)
    stats = pass3_signatures(
        prog,
        DwarfSignatureInputs(
            type_map=t_map, type_rows=t_rows, functions=(fn_row,), parameters=(p_row,)
        ),
    )
    assert not stats.error_counts
    thunk_params = thunk_fn.updates[0][1]
    assert thunk_params[0].dt.name == "void*"


def test_pass2_and_pass3_skip_phantom_and_libpng_noreturn(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from twdll._ghidra.dwarf_signatures import STATIC_NORETURN_NAMES
    from twdll.domain import DwarfVtableEntryRow

    for name in ("png_error", "png_chunk_error"):
        assert name in STATIC_NORETURN_NAMES

    _install_fake_ghidra_classes(monkeypatch)
    g_ns = _FakeNamespace("global", None)
    adaptive_fn = _FakeFunction(0x407000, "std::__stable_partition_adaptive", g_ns)
    prog = _FakeProgram(funcs=(adaptive_fn,))
    t_rows = {
        1: DwarfTypeRow(
            1, "primitive", "void", 0, None, None, None, None, None, False, False, False, True, None
        ),
        2: DwarfTypeRow(
            2,
            "class",
            "AkCtx",
            16,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "memory",
        ),
        3: DwarfTypeRow(
            3, "pointer", "AkCtx *", 8, 2, None, None, None, None, False, False, False, True, "int"
        ),
        4: DwarfTypeRow(
            4,
            "primitive",
            "long",
            8,
            None,
            None,
            None,
            None,
            None,
            False,
            False,
            False,
            True,
            "int",
        ),
    }
    ak_dt = _FakeStructDataType("global", "AkCtx", 16)
    t_map: dict[int, _FakeDataType] = {
        1: _FakeDataType("void", 0),
        2: ak_dt,
        3: _FakeDataType("AkCtx *", 8),
        4: _FakeDataType("long", 8),
    }
    dtor_fn = DwarfFunctionRow(
        10,
        1,
        None,
        None,
        None,
        "AkCtx::~AkCtx",
        2,
        1,
        0,
        1,
        False,
        True,
        False,
        True,
        False,
        None,
        None,
    )
    dtor_params = (
        DwarfParameterRow(10, 0, "this", 3, True, True, "register", 5, "RDI", None),
        DwarfParameterRow(10, 1, "__in_chrg", 4, True, False, "none", None, None, None),
    )
    pass2_stats = pass2_vtables_and_structs(
        prog,
        DwarfLayoutInputs(
            type_map=t_map,
            type_rows=t_rows,
            members=(),
            inheritances=(),
            vtables=(DwarfVtableEntryRow(2, 0, "~AkCtx", None, 10, None, False),),
            vtable_funcs={10: dtor_fn},
            vtable_params={10: dtor_params},
            globals_=(),
        ),
    )
    assert not pass2_stats.error_counts
    assert 0 in ak_dt.fields
    vt_struct = prog.dtm.added["/dwarf/AkCtx_vtbl"]
    assert 0 in vt_struct.fields

    fn_row = DwarfFunctionRow(
        20,
        1,
        0x407000,
        64,
        None,
        "std::__stable_partition_adaptive",
        None,
        None,
        None,
        0,
        False,
        False,
        False,
        False,
        False,
        None,
        None,
    )
    fn_params = (
        DwarfParameterRow(20, 0, "__first", 3, False, False, "register", 5, "RDI", None),
        DwarfParameterRow(20, 1, "__last", 3, False, False, "register", 4, "RSI", None),
        DwarfParameterRow(20, 2, None, 1, False, False, "none", None, None, None),
        DwarfParameterRow(20, 3, "__len", 4, False, False, "register", 1, "RDX", None),
        DwarfParameterRow(20, 4, "__buffer", 3, False, False, "register", 2, "RCX", None),
        DwarfParameterRow(20, 5, "__pred", 1, False, False, "none", None, None, None),
        DwarfParameterRow(20, 6, "__buffer_size", 4, False, False, "register", 8, "R8", None),
    )
    pass3_stats = pass3_signatures(
        prog,
        DwarfSignatureInputs(
            type_map=t_map, type_rows=t_rows, functions=(fn_row,), parameters=fn_params
        ),
    )
    assert not pass3_stats.error_counts
    out_params = adaptive_fn.updates[0][1]
    assert [p.name for p in out_params] == [
        "__first",
        "__last",
        "__len",
        "__buffer",
        "__buffer_size",
    ]
    assert out_params[2].storage.args[0][0].addr == prog.regs["RDX"].getAddress()
    assert out_params[3].storage.args[0][0].addr == prog.regs["RCX"].getAddress()
    assert out_params[4].storage.args[0][0].addr == prog.regs["R8"].getAddress()
