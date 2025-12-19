#!/bin/sh

pdir=~/.local/lib/python/v3/v3.14/v3.14.2/cpyhon.deps.wasm.d/cpython3.14

test -d "${pdir}" || exec sh -c '
	echo you need to prepare python for wasm somehow.
	exit 1
'

jq \
	-c \
	-n \
	--argjson data '{
		"helo":"wrld",
		"f": 42.195,
		"i": 42,
		"b": false,
		"n": null
	}' \
	--arg code 'import json; import functools; functools.reduce(
		lambda state, f: f(state),
		[
			json.dumps,
			print,
		],
		ctx,
	)' \
	'{
		ctx: $data,
		code: $code,
	}' |
	wazero \
		run \
		-env PYTHONPATH=/cross-build/wasm32-wasip1/build/lib.wasi-wasm32-3.14 \
		-mount "${pdir}:/:ro" \
		./python.wasm \
		-c 'import sys; import json; import functools; functools.reduce(
			lambda state, f: f(state),
			[
				json.load,
				lambda d: exec(d["code"], dict(ctx=d["ctx"])),
			],
			sys.stdin,
		)' |
	jq .
