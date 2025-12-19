package pyeval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrRuntime error = errors.New("runtime error")

type PyEvalInput struct {
	Code    string         `json:"code"`
	Context map[string]any `json:"context"`
}

func (i PyEvalInput) ToJson() ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input to JSON: %w", err)
	}
	return bytes, nil
}

type PyEvalResult struct {
	Result any
	Error  error
}

type ErrorDto struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type PyEvalResultDto struct {
	Result map[string]any `json:"result"`
	Error  *ErrorDto      `json:"error"`
}

func PyEvalResultDtoFromJson(j []byte) (d PyEvalResultDto, e error) {
	e = json.Unmarshal(j, &d)
	return
}

func (d PyEvalResultDto) ToResult() PyEvalResult {
	var err error
	if d.Error != nil {
		err = fmt.Errorf("%w: %s", ErrRuntime, d.Error.Message)
	}
	return PyEvalResult{
		Result: d.Result,
		Error:  err,
	}
}

type Evaluator func(context.Context, PyEvalInput) PyEvalResult
