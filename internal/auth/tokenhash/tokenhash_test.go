package tokenhash

import "testing"

func TestHashMatchesSHA256Vector(t *testing.T) {
	const want = "LPJNul-wow4m6DsqxbninhsWHlwfp0JecwQzYpOLmCQ"
	if got := Hash("hello"); got != want {
		t.Fatalf("Hash(hello) = %q, want %q", got, want)
	}
}

func TestHashIsStableAndTokenSpecific(t *testing.T) {
	const token = "token-a"
	first := Hash(token)
	second := Hash(token)
	if first != second {
		t.Fatal("hash must be stable for the same token")
	}
	if first == Hash("token-b") {
		t.Fatal("different tokens must not share a hash")
	}
}
