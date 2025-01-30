#!/bin/bash
set -eox

export CC="ccache gcc"
export CXX="ccache g++"

make -C ./lua-5.1.5 MYCFLAGS="-m32" MYLDFLAGS="-m32" mingw
"C:\Program Files\JetBrains\CLion 2024.3.2\bin\cmake\win\x64\bin\cmake.exe" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_MAKE_PROGRAM=/mingw32/bin/ninja.exe \
  -DCMAKE_C_COMPILER=/mingw32/bin/gcc.exe \
  -DCMAKE_CXX_COMPILER=/mingw32/bin/g++.exe \
  -G Ninja \
  -S $(pwd) \
  -B ./cmake-build-release-mingw-1

cmake --build ./cmake-build-release-mingw-1 --target twdll -j 22


cp ./cmake-build-release-mingw-1/libtwdll.dll "C:\Program Files (x86)\Steam\steamapps\common\Total War Rome II"
start "" rungame.lnk
