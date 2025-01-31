#!/bin/bash

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
