package wire

import "testing"

func TestAddressPolicy(t *testing.T) {
	for _, s := range []string{"127.0.0.1:3610", "127.0.0.1:0"} {
		if err := ValidateAddress(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"0.0.0.0:3610", "224.0.23.0:3610", "192.168.1.2:3610", "localhost:3610", "[::1]:3610", "8.8.8.8:3610"} {
		if err := ValidateAddress(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	// Validation only: no socket is opened in tests.
}
