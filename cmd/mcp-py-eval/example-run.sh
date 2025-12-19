#!/bin/sh

./mcp-py-eval \
	-port 12159 \
	-path2engine ~/.local/bin/python.wasm \
	-path2pythonlibs ~/.local/lib/python/v3/v3.14/v3.14.2/cpyhon.deps.wasm.d/cpython3.14
