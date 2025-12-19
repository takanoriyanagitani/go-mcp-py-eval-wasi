# Go MCP Python WASI Evaluator

This project provides a Model Context Protocol (MCP) server that safely evaluates untrusted Python code within a secure WebAssembly (WASM) sandbox.

The server's `py-eval` tool is self-documenting for language models, explaining its strict "JSON object in, JSON object out" contract and common pitfalls in its description.

## Prerequisites

A pre-built `python.wasm` module and its corresponding standard library are required. See `python.md` for a guide on building these artifacts from source.

## Usage

To run the server, you must provide a pre-built `python.wasm` module and its corresponding standard library. Refer to `example-run.sh` for a practical demonstration of how to start the server with the necessary flags.


```
