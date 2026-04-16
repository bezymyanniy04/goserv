package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	token, err := MakeJWT(userID, "secret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT() error: %v", err)
	}

	gotID, err := ValidateJWT(token, "secret")
	if err != nil {
		t.Fatalf("ValidateJWT() error: %v", err)
	}
	if gotID != userID {
		t.Errorf("got user ID %v, want %v", gotID, userID)
	}
}
