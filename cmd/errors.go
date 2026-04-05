// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

package cmd

import "errors"

// NoticeError is a non-fatal-style user notice that should still terminate the
// current command with a non-zero exit status.
type NoticeError struct {
	Message string
}

func (e *NoticeError) Error() string {
	return e.Message
}

func IsNoticeError(err error) bool {
	var target *NoticeError
	return errors.As(err, &target)
}
