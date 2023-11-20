// Package wraperr implements methods for wrapping error in Go.

package wraperr

import (
	"errors"
	"reflect"
)

// wrapError is an implementation of error that has both the
// outer and inner errors.
type wrapError struct {
	Outer error
	Inner error
}

// Wrap defines that outer wraps inner
func Wrap(outer, inner error) error {
	return &wrapError{
		Outer: outer,
		Inner: inner,
	}
}

// Wrapf wraps an error
func Wrapf(err error, format string) error {

	outMsg := "<nil>"
	if err != nil {
		outMsg = err.Error()
	}
	format = format + " : " + outMsg
	outer := errors.New(format)

	return Wrap(outer, err)
}

// Contains checks if the given error contains an error with the message
func Contains(err error, msg string) bool {
	return len(GetAll(err, msg)) > 0
}

// ContainsType checks if the given error contains an error with
// the same concrete type as v. If err is not a wrapped error, this will
// check the err itself.
func ContainsType(err error, v interface{}) bool {
	return len(GetAllType(err, v)) > 0
}

// GetType is the same as GetAllType but returns the deepest matching error.
func GetType(err error, v interface{}) error {
	es := GetAllType(err, v)
	if len(es) > 0 {
		return es[len(es)-1]
	}

	return nil
}

// GetAll gets all the errors that might be wrapped in err with the given message.
func GetAll(err error, msg string) []error {
	var result []error
	newErr := err
	for {
		if newErr.Error() == msg {
			result = append(result, newErr)
		}
		newErr = Walk(newErr)
		if newErr == nil {
			break
		}
	}

	return result
}

// GetAllType gets all the errors that are the same type as v.
//
// The order of the return value is the same as described in GetAll.
func GetAllType(err error, v interface{}) []error {

	var search string
	if v != nil {
		search = reflect.TypeOf(v).String()
	}
	var retErr []error
	for {
		nextErr := Walk(err)
		if nextErr == nil {
			break
		}
		if search == reflect.TypeOf(nextErr).String() {
			retErr = append(retErr, nextErr)
		}
	}

	return retErr
}

// Walk walks all the wrapped errors in err
func Walk(err error) error {
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case *wrapError:
		return e.Inner
	default:
		return nil
	}
}

func (w *wrapError) Error() string {
	return w.Outer.Error()
}
