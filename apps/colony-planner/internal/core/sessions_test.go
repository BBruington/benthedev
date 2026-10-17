package core

import "testing"

func TestSessionTokens(t *testing.T) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		t.Fatalf("newSessionToken: %v", err)
	}
	if len(tokenHash) != 32 {
		t.Fatalf("token hash is %d bytes, want 32", len(tokenHash))
	}
	rehashed, valid := HashSessionToken(token)
	if !valid || string(rehashed) != string(tokenHash) {
		t.Fatal("hashing the cookie value does not give the stored hash")
	}
	for _, junk := range []string{"", "not-base64!", "c2hvcnQ"} {
		if _, valid := HashSessionToken(junk); valid {
			t.Errorf("HashSessionToken(%q) accepted junk", junk)
		}
	}
}
