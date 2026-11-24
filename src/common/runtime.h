#pragma once
// runtime.h — process-level identity and ownership of the twdll runtime.
//
// One twdll runtime may own the engine hooks of a game process. Several mods can
// ship their own twdll build (under different file names), so before any hook is
// installed the runtime claims a per-process owner record. A second twdll module
// that finds another owner does not hook anything and luaopen_twdll returns
// nil + reason to its Lua caller.

#include <string>
#include <windows.h>

// Lua/global/registry/log namespace. Override at configure time with
// -DTWDLL_NAMESPACE=<name> so a mod-specific build cannot collide with the
// public build in the shared Lua state (registry key, globals, log file).
#ifndef TWDLL_NAMESPACE
#define TWDLL_NAMESPACE "twdll"
#endif

// Bumped whenever the Lua-visible API changes incompatibly, or gains functions a
// caller may need to test for. Read from Lua with <ns>.core.GetApiVersion().
#define TWDLL_API_VERSION 7

// Called from DllMain(PROCESS_ATTACH).
void runtime_set_module(HMODULE self);
HMODULE runtime_module();

// Pins the module so Lua's package.loadlib __gc (FreeLibrary) can never unmap
// code that live hooks, trampolines or registered C closures point into.
// Idempotent.
void runtime_pin_module();

// Claims the per-process owner record. Returns true if this module owns (or
// already owned) the runtime; false with a human-readable reason otherwise.
// The result is cached: ownership is kept for the lifetime of the process.
bool runtime_claim_owner(std::string& reason);

// Full path of this module (UTF-8).
std::string runtime_module_path();
