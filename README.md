# twdll
DLL that can be loaded in Rome 2 lua script.

```text
[2025-01-30 01:40:43] [INFO] Loaded library: function: 58D30310
[2025-01-30 01:40:43] [INFO] Executing mylib
[2025-01-30 01:40:43] [INFO] lib returned: function: 58D30340
[2025-01-30 01:40:43] [INFO] Executing what lib returned
[2025-01-30 01:40:43] [INFO] Result: pong
```

To load paste dll into game dir and:

```lua
local lib_logging = require "script._lib.lib_logging"
local logger = lib_logging.new_logger("scripting.txt", "DEBUG")

local ok, mylib = pcall(package.loadlib, "libtwdll.dll", "luaopen_libtwdll")

if not ok then
    logger:error("Failed to load library: " .. mylib)
else
    logger:info("Loaded library: " .. tostring(mylib))

    logger:info("Executing mylib")
    local lib_returned = mylib()
    logger:info("lib returned: " .. tostring(lib_returned))

    logger:info("Executing what lib returned")
    local result = lib_returned("ping")
    logger:info("Result: " .. tostring(result))

    assert(result == "pong", "Expected 'pong' from ping, but got: " .. tostring(result))
end
```

### Notes
Need lua 5.1.1 or 5.1.2

5.1 does no return from dostring
5.1.3 and up crashes with memory access
if you use these files from 5.1.2 into 5.1.3 it wont crash
````
lstate.h
lstate.c
ldo.c
````