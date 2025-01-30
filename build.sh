#!/bin/bash

export CC="ccache gcc"
export CXX="ccache g++"

make -C ./lua-5.1.5 MYCFLAGS="-m32" MYLDFLAGS="-m32" mingw
cmake --build . --target twdll -j 22
cp libtwdll.dll "C:\Program Files (x86)\Steam\steamapps\common\Total War Rome II"
start "" rungame.lnk