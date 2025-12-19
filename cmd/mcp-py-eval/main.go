package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pyeval "github.com/takanoriyanagitani/go-mcp-py-eval-wasi"
	"github.com/takanoriyanagitani/go-mcp-py-eval-wasi/eval/wasi"
	wa0 "github.com/takanoriyanagitani/go-mcp-py-eval-wasi/eval/wasi/wa0"
)

//go:embed doc/description.txt
var toolDescription string

const (
	defaultPort         = 12040
	readTimeoutSeconds  = 10
	writeTimeoutSeconds = 10
	maxHeaderExponent   = 20
	maxBodyBytes        = 1 * 1024 * 1024 // 1 MiB
)

var (
	port       = flag.Int("port", defaultPort, "port to listen")
	enginePath = flag.String(
		"path2engine",
		"./python.wasm",
		"path to the WASM python engine",
	)
	pythonLibsPath = flag.String(
		"path2pythonlibs",
		"",
		"path to the Python WASI standard library files (e.g., ~/.local/lib/python/v3/v3.14/v3.14.2/cpyhon.deps.wasm.d/cpython3.14)",
	)
	mem     = flag.Uint("mem", 64, "WASM memory limit in MiB")
	timeout = flag.Uint("timeout", 2000, "WASM execution timeout in milliseconds")
)

const wasmPageSizeKiB = 64
const kiBytesInMiByte = 1024
const wasmPagesInMiB = kiBytesInMiByte / wasmPageSizeKiB

func withMaxBodyBytes(h http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		h.ServeHTTP(w, r)
	})
}

func toClientError(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") {
		return "Python evaluation timed out"
	}
	if errors.Is(err, wa0.ErrUuid) {
		return "Engine configuration error"
	}
	if errors.Is(err, wa0.ErrInput) {
		return "Invalid code or context input format"
	}
	if errors.Is(err, wa0.ErrOutputJson) {
		return err.Error() // Return the full underlying error string
	}
	if errors.Is(err, wa0.ErrPythonExecution) {
		return err.Error() // Return the full underlying error string
	}
	if errors.Is(err, wa0.ErrInstantiate) {
		return "Engine instantiation failed"
	}

	return "Internal server error"
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memoryLimitPages := uint32(*mem) * wasmPagesInMiB
	evaluator, cleanup, err := wasi.NewWasiEvaluator(ctx, *enginePath, *pythonLibsPath, memoryLimitPages)
	if err != nil {
		log.Printf("failed to create WASI evaluator: %v\n", err)
		return
	}
	defer func() {
		if err := cleanup(); err != nil {
			log.Printf("failed to cleanup WASI evaluator: %v\n", err)
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "py-eval",
		Version: "v0.1.0",
		Title:   "Python Evaluator",
	}, nil)

	pyEvalTool := func(ctx context.Context, req *mcp.CallToolRequest, input pyeval.PyEvalInput) (
		*mcp.CallToolResult,
		pyeval.PyEvalResultDto,
		error,
	) {
		timeoutCtx, cancelTimeout := context.WithTimeout(ctx, time.Duration(*timeout)*time.Millisecond)
		defer cancelTimeout()

		// Run the evaluator in a goroutine so we can select on its completion or a timeout.
		resultChan := make(chan pyeval.PyEvalResult, 1)
		go func() {
			resultChan <- evaluator(timeoutCtx, input)
		}()

		select {
		case <-timeoutCtx.Done():
			// This case is triggered if the context times out before the evaluator returns.
			log.Printf("Error processing code: Python evaluation timed out after %dms", *timeout)
			clientError := toClientError(timeoutCtx.Err())
			return nil, pyeval.PyEvalResultDto{
				Result: make(map[string]any),
				Error: &pyeval.ErrorDto{
					Code:    -1,
					Message: clientError,
				},
			}, nil

		case result := <-resultChan:
			// This case is triggered when the evaluator returns a result.
			if result.Error != nil {
				// The evaluator itself returned an error (e.g., non-zero exit, invalid JSON).
				// We still check for DeadlineExceeded here in case the evaluator
				// correctly propagated it.
				if errors.Is(result.Error, context.DeadlineExceeded) {
					log.Printf("Error processing code: Python evaluation timed out after %dms", *timeout)
				} else {
					log.Printf("Error processing code: %v", result.Error)
				}
				clientError := toClientError(result.Error)
				return nil, pyeval.PyEvalResultDto{
					Result: make(map[string]any),
					Error: &pyeval.ErrorDto{
						Code:    -1,
						Message: clientError,
					},
				}, nil
			}
			// The evaluator returned a successful result.
			return nil, pyeval.PyEvalResultDto{
					Result: result.Result.(map[string]any),
					Error:  nil,
				},
				nil
		}
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:         "py-eval",
		Title:        "Python Evaluator",
		Description:  toolDescription,
		InputSchema:  nil, // Inferred by AddTool
		OutputSchema: nil, // Inferred by AddTool
	}, pyEvalTool)

	address := fmt.Sprintf(":%d", *port)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)

	httpServer := &http.Server{
		Addr:           address,
		Handler:        withMaxBodyBytes(mcpHandler, maxBodyBytes),
		ReadTimeout:    readTimeoutSeconds * time.Second,
		WriteTimeout:   writeTimeoutSeconds * time.Second,
		MaxHeaderBytes: 1 << maxHeaderExponent,
	}

	log.Printf("Ready to start HTTP MCP server. Listening on %s\n", address)
	err = httpServer.ListenAndServe()
	if err != nil {
		log.Printf("Failed to listen and serve: %v\n", err)
		return
	}
}
