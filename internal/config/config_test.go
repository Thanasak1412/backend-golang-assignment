package config

import "testing"

func TestSecretRequired(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing secret must fail")
	}
	t.Setenv("JWT_SECRET", "REPLACE_WITH_RANDOM_64_HEX_CHAR_SECRET")
	if _, err := Load(); err == nil {
		t.Fatal("example placeholder must fail")
	}
}
