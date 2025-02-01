# twdll

`twdll` is a sample project demonstrating how to call *Total War: Rome II*'s Lua stack directly from C. The main challenge was manually determining the correct Lua version used by the game to ensure proper execution.

## What This Project Does

- **Directly Executes *Rome II*'s Lua State in C** 
  - – Enables calling and manipulating the game's Lua stack from native C code.
- **Allows Native Calls with Lua State** 
  - – Ensures that C functions can execute within the Lua execution context without breaking it.
- **Loads a DLL in Lua** 
  - – Demonstrates how to inject a native library into the game's scripting environment.

### Notes

- Requires **Lua 5.1.1** or **Lua 5.1.2** for proper functionality.
- **Lua 5.1** does not return from `dostring`.
- **Lua 5.1.3 and later** crash due to memory access issues.
- To prevent crashes in **Lua 5.1.3**, you can replace the following files with those from **Lua 5.1.2**:  
    ```
  lstate.h
    lstate.c
    ldo.c
  ```
  