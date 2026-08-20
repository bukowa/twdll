# Cross-compile twdll (32-bit MSVC-ABI DLL) on Linux with clang-cl
# Requires MSVC CRT + Windows SDK laid out by xwin at $XWIN_ROOT (default ~/xwin).
set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR x86)

if(NOT DEFINED ENV{XWIN_ROOT})
    set(XWIN "$ENV{HOME}/xwin")
else()
    set(XWIN "$ENV{XWIN_ROOT}")
endif()

set(CMAKE_C_COMPILER   clang-cl)
set(CMAKE_CXX_COMPILER clang-cl)
set(CMAKE_C_COMPILER_TARGET   i686-pc-windows-msvc)
set(CMAKE_CXX_COMPILER_TARGET i686-pc-windows-msvc)

# xwin header roots (clang-cl consumes /imsvc as system-include, silencing SDK warnings).
set(_xwin_incs
    "/imsvc${XWIN}/crt/include"
    "/imsvc${XWIN}/sdk/include/ucrt"
    "/imsvc${XWIN}/sdk/include/shared"
    "/imsvc${XWIN}/sdk/include/um"
    "/imsvc${XWIN}/sdk/include/winrt")
string(JOIN " " _xwin_inc_flags ${_xwin_incs})
set(CMAKE_C_FLAGS_INIT   "--target=i686-pc-windows-msvc ${_xwin_inc_flags}")
set(CMAKE_CXX_FLAGS_INIT "--target=i686-pc-windows-msvc ${_xwin_inc_flags}")

# xwin x86 (32-bit) lib roots.
set(_xwin_libs
    "/libpath:${XWIN}/crt/lib/x86"
    "/libpath:${XWIN}/sdk/lib/ucrt/x86"
    "/libpath:${XWIN}/sdk/lib/um/x86")
string(JOIN " " _xwin_lib_flags ${_xwin_libs})
foreach(_t EXE SHARED MODULE)
    set(CMAKE_${_t}_LINKER_FLAGS_INIT "${_xwin_lib_flags}")
endforeach()
