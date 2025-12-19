package wa0pyeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/google/uuid"
	pyeval "github.com/takanoriyanagitani/go-mcp-py-eval-wasi"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
)

var (
	ErrInstantiate     error = errors.New("internal error: invalid evaluation engine")
	ErrUuid            error = errors.New("internal error: unable to configure engine")
	ErrInput           error = errors.New("input error: invalid json")
	ErrOutputJson      error = errors.New("output error: invalid json")
	ErrPythonExecution error = errors.New("python execution error")
)

type RuntimeConfig struct{ wazero.RuntimeConfig }

func RuntimeConfigNewDefault() RuntimeConfig {
	return RuntimeConfig{
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithCloseOnContextDone(true),
	}
}

func (c RuntimeConfig) WithPageLimit(memoryLimitPages uint32) RuntimeConfig {
	return RuntimeConfig{
		RuntimeConfig: c.RuntimeConfig.WithMemoryLimitPages(memoryLimitPages),
	}
}

func (c RuntimeConfig) ToRuntime(ctx context.Context) WasmRuntime {
	rtm := wazero.NewRuntimeWithConfig(ctx, c.RuntimeConfig)
	return WasmRuntime{Runtime: rtm}
}

type WasmRuntime struct{ wazero.Runtime }

type UUID struct{ uuid.UUID }

func (u UUID) String() string { return u.UUID.String() }

func (u UUID) ToInstanceName() string { return "instance-" + u.String() }

func UUID7() (UUID, error) {
	uid, e := uuid.NewV7()
	if e != nil {
		return UUID{}, fmt.Errorf("failed to create V7 UUID: %w", e)
	}
	return UUID{UUID: uid}, nil
}

func (r WasmRuntime) Close(ctx context.Context) error {
	err := r.Runtime.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close runtime: %w", err)
	}
	return nil
}

func (r WasmRuntime) Compile(ctx context.Context, wasm []byte) (Compiled, error) {
	cmod, e := r.Runtime.CompileModule(ctx, wasm)
	if e != nil {
		return Compiled{}, fmt.Errorf("failed to compile module: %w", e)
	}
	return Compiled{CompiledModule: cmod}, nil
}

func (r WasmRuntime) Instantiate(ctx context.Context, cmod Compiled, cfg WasmConfig) (WasmInstance, error) {
	ins, e := r.Runtime.InstantiateModule(ctx, cmod.CompiledModule, cfg.ModuleConfig)
	if e != nil {
		return WasmInstance{}, fmt.Errorf("failed to instantiate module: %w", e)
	}
	return WasmInstance{Module: ins}, nil
}

func (r WasmRuntime) ToEvaluator(ctx context.Context, cmod Compiled, cfg WasmConfig) pyeval.Evaluator {
	return func(ctx context.Context, input pyeval.PyEvalInput) pyeval.PyEvalResult {
		newCfg, e := cfg.WithAutoID()
		if nil != e {
			log.Printf("Failed to generate UUID for WASM instance: %v", e)
			return pyeval.PyEvalResult{
				Error: ErrUuid,
			}
		}

		inputJson, e := input.ToJson()
		if nil != e {
			log.Printf("Failed to serialize input to JSON: %v", e)
			return pyeval.PyEvalResult{
				Error: ErrInput,
			}
		}

		var output bytes.Buffer

		cfgWithStdio := newCfg.
			WithReader(bytes.NewReader(inputJson)).
			WithWriter(&output)

		ins, e := r.Instantiate(ctx, cmod, cfgWithStdio)
		if e != nil {
			// Check for timeout before wrapping in a generic error.
			if errors.Is(e, context.DeadlineExceeded) {
				return pyeval.PyEvalResult{Error: context.DeadlineExceeded}
			}
			return pyeval.PyEvalResult{
				Error: fmt.Errorf("%w: %w", ErrInstantiate, e),
			}
		}

		// Explicitly close the instance to catch any exit errors.
		if closeErr := ins.Close(ctx); closeErr != nil {
			// Check for timeout before wrapping in a generic error.
			if errors.Is(closeErr, context.DeadlineExceeded) {
				return pyeval.PyEvalResult{Error: context.DeadlineExceeded}
			}
			var exitErr *sys.ExitError
			if errors.As(closeErr, &exitErr) {
				return pyeval.PyEvalResult{
					Error: fmt.Errorf("%w: script exited with code %d", ErrPythonExecution, exitErr.ExitCode()),
				}
			}
			// Another type of error occurred on close.
			return pyeval.PyEvalResult{
				Error: fmt.Errorf("error closing wasm module: %w", closeErr),
			}
		}

		// Exit code was 0, now validate the output.
		scriptOutput := output.Bytes()
		trimmedOutput := bytes.TrimSpace(scriptOutput)

		if len(trimmedOutput) == 0 {
			// Empty or whitespace-only output is an error, as a JSON object is expected.
			return pyeval.PyEvalResult{
				Error: fmt.Errorf("%w: script produced no output, but a JSON object was expected", ErrOutputJson),
			}
		}

		if !json.Valid(trimmedOutput) {
			return pyeval.PyEvalResult{
				Error: fmt.Errorf("%w: script output was not valid JSON. Raw output: %s", ErrOutputJson, string(trimmedOutput)),
			}
		}

		var jsonObject map[string]any
		if err := json.Unmarshal(trimmedOutput, &jsonObject); err != nil {
			return pyeval.PyEvalResult{
				Error: fmt.Errorf("%w: script output was valid JSON but not an object (map[string]any). Raw output: %s", ErrOutputJson, string(trimmedOutput)),
			}
		}

		// The output is a valid JSON object, return it.
		return pyeval.PyEvalResult{
			Result: jsonObject,
			Error:  nil,
		}
	}
}

type Compiled struct{ wazero.CompiledModule }

type WasmConfig struct{ wazero.ModuleConfig }

func (c WasmConfig) WithName(name string) WasmConfig {
	return WasmConfig{
		ModuleConfig: c.ModuleConfig.WithName(name),
	}
}

func (c WasmConfig) WithID(id UUID) WasmConfig {
	name := id.ToInstanceName()
	return c.WithName(name)
}

func (c WasmConfig) WithAutoID() (WasmConfig, error) {
	id, e := UUID7()
	if e != nil {
		return WasmConfig{}, fmt.Errorf("failed to get auto ID: %w", e)
	}
	neo := c.WithID(id)
	return neo, nil
}

func (c WasmConfig) WithReader(rdr io.Reader) WasmConfig {
	return WasmConfig{
		ModuleConfig: c.ModuleConfig.WithStdin(rdr),
	}
}

func (c WasmConfig) WithWriter(wtr io.Writer) WasmConfig {
	return WasmConfig{
		ModuleConfig: c.ModuleConfig.WithStdout(wtr),
	}
}

type WasmInstance struct{ api.Module }

func (i WasmInstance) Close(ctx context.Context) error {
	err := i.Module.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close module: %w", err)
	}
	return nil
}
