package auth

import "testing"

func TestNoopValidator_AllowsAll(t *testing.T) {
	var v Validator = NoopValidator{}

	if err := v.CheckRoomLimit(0); err != nil {
		t.Errorf("CheckRoomLimit(0): got %v, want nil", err)
	}
	if err := v.CheckRoomLimit(1_000_000); err != nil {
		t.Errorf("CheckRoomLimit(1M): got %v, want nil", err)
	}
	if err := v.CheckPeerLimit(0); err != nil {
		t.Errorf("CheckPeerLimit(0): got %v, want nil", err)
	}
	if err := v.CheckPeerLimit(1_000_000); err != nil {
		t.Errorf("CheckPeerLimit(1M): got %v, want nil", err)
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	if ErrLicenseExpired == ErrRoomLimitExceeded ||
		ErrLicenseExpired == ErrPeerLimitExceeded ||
		ErrRoomLimitExceeded == ErrPeerLimitExceeded {
		t.Fatal("sentinel errors must be distinct values")
	}
}
