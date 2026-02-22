package cli

import "errors"

// UserError marks an error as a user/configuration mistake rather than an
// unexpected system failure. Execute uses this to suppress the --debug hint
// (which is unhelpful for config problems) and to return exit code 1 instead
// of exit code 2.
type UserError struct{ cause error }

func (e UserError) Error() string { return e.cause.Error() }
func (e UserError) Unwrap() error { return e.cause }

// UserErr wraps err as a UserError. It returns nil when err is nil.
func UserErr(err error) error {
	if err == nil {
		return nil
	}
	return UserError{cause: err}
}

// IsUserError reports whether err is (or wraps) a UserError.
func IsUserError(err error) bool {
	var u UserError
	return errors.As(err, &u)
}
