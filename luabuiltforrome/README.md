if you want to compile lua into luac in a format that Rome 2 can execute (no idea about other TW games)
you have to use luac that uses lua #define LUA_NUMBER    float and not default #define LUA_NUMBER    double

i tried official lua win32 library and it couldn't run the coreutils.luac provided by default Rome2
so I checked for headers of this file and turned out its using 000A  04                 size of number (bytes)

so i downloaded lua source code from git, but it didnt build (lua 5.1)
so if you ever want to run it get source from this file https://www.lua.org/ftp/
then you have to build for win32
you need gcc and few other libs i recommend installing via pacman msys
make MYCFLAGS="-m32" MYLDFLAGS="-m32" mingw