package auth

import (
	"os"
	"testing"
)

// The override exists for a long-lived key on the wire, so only a true boolean
// opens it — `=0` and `=false` must keep the guard closed, and anything the flag
// cannot parse fails safe.
func TestInsecureAPISecretAllowed(t *testing.T) {
	cases := []struct {
		set  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"no", false},  // not a bool: fail safe rather than guess
		{"yes", false}, // ditto
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{" true ", true}, // a trailing space in .env must not flip the meaning
	}
	for _, c := range cases {
		t.Setenv(AllowInsecureAPISecretEnv, c.set)
		if c.set == "" {
			os.Unsetenv(AllowInsecureAPISecretEnv)
		}
		if got := InsecureAPISecretAllowed(); got != c.want {
			t.Errorf("InsecureAPISecretAllowed() with %q = %v, want %v", c.set, got, c.want)
		}
	}
}
