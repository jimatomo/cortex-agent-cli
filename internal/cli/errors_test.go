package cli

import (
	"errors"
	"fmt"
	"testing"
)

func TestUserErr_NilIsNil(t *testing.T) {
	if UserErr(nil) != nil {
		t.Error("UserErr(nil) should return nil")
	}
}

func TestIsUserError(t *testing.T) {
	base := fmt.Errorf("something went wrong")

	if IsUserError(base) {
		t.Error("plain error should not be a UserError")
	}

	wrapped := UserErr(base)
	if !IsUserError(wrapped) {
		t.Error("UserErr-wrapped error should be a UserError")
	}
}

func TestUserError_Unwrap(t *testing.T) {
	base := fmt.Errorf("underlying cause")
	wrapped := UserErr(base)

	if !errors.Is(wrapped, base) {
		t.Error("errors.Is should find the original cause through Unwrap")
	}
	if wrapped.Error() != base.Error() {
		t.Errorf("Error() = %q, want %q", wrapped.Error(), base.Error())
	}
}

func TestIsUserError_ThroughWrapper(t *testing.T) {
	base := fmt.Errorf("config missing")
	user := UserErr(base)
	outer := fmt.Errorf("context: %w", user)

	if !IsUserError(outer) {
		t.Error("IsUserError should find UserError through fmt.Errorf wrapping")
	}
}
