package evaluate

import (
	"math"
	"testing"
)

func TestEval(t *testing.T) {
	tests := []struct {
		formula string
		x       float64
		want    float64
		wantErr bool
	}{
		{"", 100, 100, false},
		{"x * 0.1", 250, 25, false},
		{"x + 100", 50, 150, false},
		{"x - 5", 10, 5, false},
		{"x / 2", 8, 4, false},
		{"(x - 20) * 0.5", 100, 40, false},
		{"x * 1.8 + 32", 0, 32, false},
		{"(x + 10) / 3", 20, 10, false},
		{"x * 2 - 10", 25, 40, false},
		{"x * 0.001", 5000, 5, false},
		{"x / 3", 10, 10.0 / 3.0, false},
		{"-x", 5, -5, false},
		{"x * (1 + 2)", 10, 30, false},
		{"x / 0", 10, 0, true},
	}

	for _, tt := range tests {
		got, err := Eval(tt.formula, tt.x)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Eval(%q, %v) expected error, got %v", tt.formula, tt.x, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Eval(%q, %v) unexpected error: %v", tt.formula, tt.x, err)
			continue
		}
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("Eval(%q, %v) = %v, want %v", tt.formula, tt.x, got, tt.want)
		}
	}
}
