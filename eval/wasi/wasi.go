package wasi

import (
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"log"
	"os"

	pyeval "github.com/takanoriyanagitani/go-mcp-py-eval-wasi"
	wa0 "github.com/takanoriyanagitani/go-mcp-py-eval-wasi/eval/wasi/wa0"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed py/executor.py
var scriptExecutor string

// NewWasiEvaluator creates a new Evaluator that uses the WASI WebAssembly module.
// It loads the WASM binary from the given wasmFilePath.
func NewWasiEvaluator(
	ctx context.Context,
	wasmFilePath string,
	pythonLibsPath string,
	memoryLimitPages uint32,
) (pyeval.Evaluator, func() error, error) {
	wasmBinary, err := os.ReadFile(wasmFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read WASM file from %s: %w", wasmFilePath, err)
	}

	rcfg := wa0.RuntimeConfigNewDefault().
		WithPageLimit(memoryLimitPages)
	r := rcfg.ToRuntime(ctx)
	cleanup := func() error { return r.Close(context.Background()) }

	_, err = wasi_snapshot_preview1.Instantiate(ctx, r.Runtime)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to instantiate wasi_snapshot_preview1: %w", err)
	}

	compiled, err := r.Compile(ctx, wasmBinary)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile WASM module from %s: %w", wasmFilePath, err)
	}

	log.Printf("WASM module from %s compiled successfully.", wasmFilePath)

	// Configure WASI for Python libs if path is provided
	var pyLibFs fs.FS
	if len(pythonLibsPath) > 0 {
		pyLibFs = os.DirFS(pythonLibsPath)
		log.Printf("Mounting Python libraries from %s to / in WASM", pythonLibsPath)
	}

	moduleConfig := wazero.NewModuleConfig().
		WithSysWalltime().
		WithSysNanotime().
		WithSysNanosleep().
		WithStderr(log.Writer())

	if pyLibFs != nil {
		moduleConfig = moduleConfig.
			WithFS(pyLibFs).
			WithEnv("PYTHONPATH", "/").
			WithArgs("python", "-c", scriptExecutor)
	}

	config := wa0.WasmConfig{
		ModuleConfig: moduleConfig,
	}

	return r.ToEvaluator(ctx, compiled, config), cleanup, nil
}
