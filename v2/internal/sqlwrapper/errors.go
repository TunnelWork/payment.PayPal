package sqlwrapper

import "errors"

var (
	ErrNilPointer     = errors.New("sqlwrapper: illegal nil pointer")
	ErrBundledPayment = errors.New("sqlweapper: unexpected bundled order")
)
