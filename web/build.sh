#!/bin/bash
set -e

cd "$(dirname "$0")/.."

# Step 1: Build native compiler if needed
if [ ! -f build/rtg ]; then
  echo "Building native compiler..."
  go build -o build/rtg ./std/compiler/
fi

# Step 2: Compile WASM compiler (without embedded std to reduce size)
echo "Compiling WASM compiler..."
./build/rtg -T wasi/wasm32 -tags no_embed_std -o web/compiler.wasm ./std/compiler/
echo "Generated web/compiler.wasm ($(wc -c < web/compiler.wasm) bytes)"

# Step 3: Bundle everything into a single playground.html
echo "Bundling..."
node web/bundle.js

echo "Build complete!"
