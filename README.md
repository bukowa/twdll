
To load paste dll into game dir and:

```lua
local ok, mylib = pcall(package.loadlib,"libtwdll.dll", "luaopen_libtwdll")
```

### warning 
: right now the dll crashes when executing