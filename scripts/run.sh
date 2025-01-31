#!/bin/bash

# Check for arguments
if [ "$1" == "luasourcediff" ]; then
    # Step 1: LuaSourceDiff Clone and Directory Setup
    mkdir -p include
    cd include || exit

    git clone git@github.com:bukowa/luasourcediff.git repo
    cd repo || exit

    for tag in 5.1 5.1.1 5.1.2 5.1.3 5.1.4 5.1.5; do
        git checkout "$tag"
        mkdir -p "../lua-ver-$tag"  # Rename the directory
        cp -r . "../lua-ver-$tag"
        rm -rf "../lua-ver-$tag/.git"  # Remove .git directory
    done

    cd ..
    rm -rf repo

elif [ "$1" == "clion" ]; then
    # Step 2: CLion Toolchains Configuration
    mkdir -p .clion/options/windows
    cp "$USERPROFILE/AppData\Roaming\JetBrains\CLion2024.3\options\windows\toolchains.xml" .clion/./options/windows/

else
    echo "Usage: $0 {luasourcediff|clion}"
    exit 1
fi
