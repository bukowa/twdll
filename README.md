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
- `CA` uses this method of table insertion that doesn't work in our lua VM

  ```lua
  -- CA method
  events[event][#events[event]+1] = func
  
  -- REQUIRED method
  table.insert(events[event], #events[event], func)
  ```
  It happens here https://github.com/bukowa/twdll/blob/bc9b2092009a261ec7e2f0fd7171befe229b0344/include/lua-ver-5.1.2/src/ltable.c#L425
  ```
  /*
  Exception 0xc0000005 encountered at address 0x23af116e: Access violation writing location 0x68801ccc
  
  ** inserts a new key into a hash table; first, check whether key's main
  ** position is free. If not, check whether colliding node is in its main
  ** position or not: if it is not, move colliding node to an empty place and
  ** put new key in its main position; otherwise (colliding node is in its main
  ** position), new key goes to an empty position.
  */
  static TValue *newkey (lua_State *L, Table *t, const TValue *key) {
  
  (lldb) memory read 0x23b5116e
  0x23b5116e: 89 50 08 8b 4d fc 8b 55 10 8b 42 04 89 41 0c 8b  �P.�M��U.�B.�A.�
  0x23b5117e: 4d 10 83 79 04 04 7c 2a 8b 55 10 8b 02 0f b6 48  M.�y..|*�U.�..�H
  
  (lldb) memory read 0x68801ccc -c1000
  0x68801ccc: 00 00 00 00 00 00 00 00 00 00 00 00 69 6e 76 61  ............inva
  0x68801cdc: 6c 69 64 20 6b 65 79 20 74 6f 20 27 6e 65 78 74  lid key to 'next
  0x68801cec: 27 00 00 00 74 61 62 6c 65 20 6f 76 65 72 66 6c  '...table overfl
  0x68801cfc: 6f 77 00 00 74 61 62 6c 65 20 69 6e 64 65 78 20  ow..table index
  0x68801d0c: 69 73 20 6e 69 6c 00 00 74 61 62 6c 65 20 69 6e  is nil..table in
  0x68801d1c: 64 65 78 20 69 73 20 4e 61 4e 00 00 25 2e 31 34  dex is NaN..%.14
  0x68801d2c: 67 00 00 00 69 6e 64 65 78 00 00 00 6c 6f 6f 70  g...index...loop
  0x68801d3c: 20 69 6e 20 67 65 74 74 61 62 6c 65 00 00 00 00   in gettable....
  0x68801d4c: 6c 6f 6f 70 20 69 6e 20 73 65 74 74 61 62 6c 65  loop in settable
  0x68801d5c: 00 00 00 00 73 74 72 69 6e 67 20 6c 65 6e 67 74  ....string lengt
  0x68801d6c: 68 20 6f 76 65 72 66 6c 6f 77 00 00 67 65 74 20  h overflow..get
  0x68801d7c: 6c 65 6e 67 74 68 20 6f 66 00 00 00 27 66 6f 72  length of...'for
  0x68801d8c: 27 20 69 6e 69 74 69 61 6c 20 76 61 6c 75 65 20  ' initial value
  0x68801d9c: 6d 75 73 74 20 62 65 20 61 20 6e 75 6d 62 65 72  must be a number

  ```