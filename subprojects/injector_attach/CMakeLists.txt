cmake_minimum_required(VERSION 3.10)

project(injector_attach LANGUAGES CXX)

# Add the executable target from our main.cpp file
add_executable(injector_attach main.cpp)

# Link against Windows libraries needed for process and thread manipulation
if(WIN32)
    target_link_libraries(injector_attach PRIVATE kernel32.lib)
endif()

add_dependencies(injector_attach memalochook_attach)

add_custom_command(
    TARGET injector_attach POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy
    $<TARGET_FILE:memalochook_attach>
    $<TARGET_FILE_DIR:injector_attach>/memalochook.dll
    COMMENT "Copying memalochook.dll for injector_attach"
)


# Optional: Install the executable to a 'bin' directory in the build output
install(TARGETS injector_attach DESTINATION bin)
