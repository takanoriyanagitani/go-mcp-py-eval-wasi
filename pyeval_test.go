package pyeval

import (
	"errors"
	"reflect"
	"testing"
)

func TestPyEvalResultDtoFromJson(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    PyEvalResultDto
		wantErr bool
	}{
		{
			name:    "valid result",
			jsonStr: `{"result": {"key": "value"}, "error": null}`,
			want: PyEvalResultDto{
				Result: map[string]any{"key": "value"},
				Error:  nil,
			},
			wantErr: false,
		},
		{
			name:    "valid error",
			jsonStr: `{"result": null, "error": {"code": 1, "message": "Python error"}}`,
			want: PyEvalResultDto{
				Result: nil,
				Error: &ErrorDto{
					Code:    1,
					Message: "Python error",
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid json string",
			jsonStr: `invalid json`,
			want:    PyEvalResultDto{},
			wantErr: true,
		},
		{
			name:    "result is not an object",
			jsonStr: `{"result": 42, "error": null}`,
			want:    PyEvalResultDto{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PyEvalResultDtoFromJson([]byte(tt.jsonStr))
			if (err != nil) != tt.wantErr {
				t.Errorf("PyEvalResultDtoFromJson() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PyEvalResultDtoFromJson() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPyEvalResultDtoToResult(t *testing.T) {
	tests := []struct {
		name string
		dto  PyEvalResultDto
		want PyEvalResult
	}{
		{
			name: "success with result object",
			dto: PyEvalResultDto{
				Result: map[string]any{"status": "ok"},
				Error:  nil,
			},
			want: PyEvalResult{
				Result: map[string]any{"status": "ok"},
				Error:  nil,
			},
		},
		{
			name: "error from dto",
			dto: PyEvalResultDto{
				Result: map[string]any{},
				Error: &ErrorDto{
					Code:    10,
					Message: "Script failed",
				},
			},
			want: PyEvalResult{
				Result: map[string]any{},
				Error:  errors.New("runtime error: Script failed"),
			},
		}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dto.ToResult()

			if !reflect.DeepEqual(got.Result, tt.want.Result) {
				t.Errorf("ToResult() got.Result = %v, want.Result = %v", got.Result, tt.want.Result)
			}

			if (got.Error != nil && tt.want.Error == nil) ||
				(got.Error == nil && tt.want.Error != nil) ||
				(got.Error != nil && tt.want.Error != nil && got.Error.Error() != tt.want.Error.Error()) {
				t.Errorf("ToResult() got.Error = %v, want.Error = %v", got.Error, tt.want.Error)
			}
		})
	}
}
