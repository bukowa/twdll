# Current Challenges and Problems

This document outlines the precise technical challenges and potential issues encountered during the development and integration of `twdll`. `twdll` is a C++ Dynamic Link Library (DLL) designed to inject into a game process and extend its functionality. Our core activities involve two main areas:

1.  **Extending the Game's Lua Environment:** We register our custom C++ functions and interfaces (e.g., for game units, characters) directly into the game's *existing and running Lua environment*. This allows Lua scripts, potentially for modding or advanced gameplay logic, to interact with game internals through our exposed C++ API.
2.  **Interacting with Game Internals via Dynamic Code Modification:** To access and modify game functions that are not publicly exported or easily accessible, we employ dynamic code modification (hooking) using libraries like `MinHook`. This allows us to intercept and alter the behavior of internal game routines.

These fundamental operations introduce specific and significant challenges:

## 1. Lua Version Compatibility

**Problem:** Ensuring `twdll` operates stably within the game's Lua environment requires absolute compatibility between the Lua C API `twdll` uses and the Lua C API the game itself uses.

**What we are doing (and why it's a challenge):** We are a C++ DLL that injects into the game process. The game has its own embedded Lua environment. Crucially, the game *does not expose its internal Lua DLL for us to link against*. This forces us to link `twdll` against *our own separate Lua DLL* (e.g., one of the `lua-ver-5.1.x` versions found in our `include` directory). When the game calls our `luaopen_twdll` function, it passes us a `lua_State* L` pointer that belongs to *its* Lua runtime. However, when our C++ code then calls Lua C API functions (e.g., `lua_pushstring`, `luaL_register`) to manipulate this `lua_State* L`, it is calling functions from *our own, separately linked Lua runtime*, not the game's.

**Why this is a challenge (in practical terms):** This creates a highly problematic scenario. We are receiving a pointer to a data structure (`lua_State`) that is managed and understood by the game's Lua runtime, but we are attempting to operate on that data structure using functions from a *different, independently linked Lua runtime* (ours). If our linked Lua runtime is a different version or build than the game's, the functions we call will interpret the `lua_State` structure differently than how the game's Lua runtime expects it. This inevitably leads to immediate and severe Application Binary Interface (ABI) mismatches, memory corruption, and crashes, as our Lua functions try to read or write to memory locations within the game's `lua_State` that they misinterpret.

**Potential Issues if Mismatched:**
*   **Application Binary Interface (ABI) Mismatches:** The internal memory layout of Lua structures (like `lua_State` or `lua_Debug`) or the calling conventions for Lua C API functions can vary between different Lua versions or builds. If `twdll`'s linked Lua runtime expects one ABI and the game's Lua runtime provides another, our C++ code will misinterpret Lua's internal state, leading to memory corruption, incorrect argument handling, or immediate crashes.
*   **API Inconsistencies:** While less common within minor versions, certain Lua C API functions might have subtle behavioral differences, be deprecated, or even be entirely absent in the game's specific Lua version, causing `twdll` to fail or behave unexpectedly.
*   **Memory Management Conflicts:** If `twdll`'s linked Lua runtime and the game's Lua runtime are built with different C runtime libraries or use distinct memory allocators, managing memory across this boundary can result in double-frees, memory leaks, or heap corruption, as one component tries to free memory allocated by another using an incompatible mechanism.

## 2. Memory Errors and Leaks

**Problem:** Given `twdll`'s deep integration and dynamic nature, preventing memory errors and leaks is a critical and complex task, even if the primary functionality appears to work.

**Why this is a challenge:** Our DLL operates at a low level, directly manipulating game memory and relying on Lua's garbage collection for certain cleanup tasks. This requires precise control over resource lifecycles.

**Specific Areas of Concern:**
*   **Resource Lifecycle Management with Lua's Garbage Collection:** We expose C++ resources and functionality to Lua, and we specifically utilize Lua's garbage collection mechanism (via the `__gc` metamethod on Lua userdata) to trigger the cleanup of these associated C++ resources. This approach is inherently complex because:
    *   Lua's garbage collection is non-deterministic; we cannot precisely control *when* the `__gc` metamethod will be called.
    *   Ensuring that C++ resources are correctly allocated, remain valid while Lua holds references, and are deallocated *exactly once* and at the appropriate time (neither too early, too late, nor multiple times) is extremely difficult. Failures here directly lead to memory leaks (resources not freed) or use-after-free errors/crashes (resources freed prematurely).
*   **Dynamic Code Modification Cleanup:** We employ dynamic code modification (hooking) to alter the game's execution flow. This involves injecting our code into the game's process memory. The setup of these hooks is one challenge, but the *cleanup* is even more critical. Hooks must be disabled and removed cleanly and safely when `twdll` is unloaded (e.g., via `DllMain`) or when the Lua state is being shut down. Improper cleanup can lead to:
    *   **Memory Corruption:** If hook trampolines or related structures are not properly restored or freed.
    *   **Access Violations:** If the game attempts to execute code at an address where a hook was partially removed or corrupted.
    *   **System Instability:** If the game's execution flow remains altered in an invalid state after `twdll` is gone.
*   **General C++ Memory Management:** Beyond the specific interactions with Lua and hooking, `twdll` is a C++ application. All raw memory allocations (`new`/`malloc`) and deallocations (`delete`/`free`) within our C++ code must be perfectly balanced and correctly managed to prevent conventional memory leaks or heap corruption over the game's runtime.

## Build Commands

```sh
cmake --preset vs2022-win32-ninja
cmake --build --preset vs2022-win32-ninja
```
