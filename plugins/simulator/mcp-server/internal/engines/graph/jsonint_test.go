package graph

import (
	"encoding/json"
	"testing"
)

// TestJSONIntUnmarshal verifies jsonInt accepts both integer and fractional JSON
// numbers, rounding fractions to nearest (half away from zero, per math.Round).
func TestJSONIntUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want jsonInt
	}{
		{"0", 0},
		{"42", 42},
		{"-7", -7},
		{"-1543.58", -1544},
		{"1543.49", 1543},
		{"2.5", 3},
		{"-2.5", -3},
	}
	for _, c := range cases {
		var got jsonInt
		if err := json.Unmarshal([]byte(c.in), &got); err != nil {
			t.Fatalf("Unmarshal(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Unmarshal(%q) = %d, want %d", c.in, int(got), int(c.want))
		}
	}
}

// TestPositionFractionalParses is the regression guard: a position object with a
// fractional coordinate must now parse. With plain `int` fields this failed with
// "json: cannot unmarshal number -1543.58 into Go ... of type int", which broke
// pull/push and the paginated readers for the whole layer.
func TestPositionFractionalParses(t *testing.T) {
	var p struct {
		X jsonInt `json:"x"`
		Y jsonInt `json:"y"`
	}
	if err := json.Unmarshal([]byte(`{"x":-1543.58,"y":42}`), &p); err != nil {
		t.Fatalf("fractional position should parse, got error: %v", err)
	}
	if p.X != -1544 || p.Y != 42 {
		t.Errorf("got (%d,%d), want (-1544,42)", int(p.X), int(p.Y))
	}
}
