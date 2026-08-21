package account

import "testing"

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	email, err := NormalizeEmail("  DEV@Example.com ")
	if err != nil {
		t.Fatalf("NormalizeEmail returned an error: %v", err)
	}
	if email != "dev@example.com" {
		t.Fatalf("NormalizeEmail returned %q", email)
	}
}

func TestNormalizeEmailRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeEmail("not an email"); err == nil {
		t.Fatal("NormalizeEmail accepted an invalid address")
	}
}
