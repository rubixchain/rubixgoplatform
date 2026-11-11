package fullnode

import "errors"

// Failure classification. A verdict (this node judged the transaction invalid
// on its own evidence) stops retries, is dead-lettered and fails the waiting
// transactions; a transient failure (e.g. an unreachable peer) is retried. Errors
// outside validation are unclassified and retried. Inside validation anything
// not tagged errDependencyTimeout is a verdict, so peer calls must tag errors.

var (
	// errValidationFailed marks a transaction this node has judged invalid on
	// its own evidence. Deterministic: another attempt reaches the same verdict.
	errValidationFailed = errors.New("transaction failed validation")

	// errDependencyTimeout marks a transient failure that says nothing about the
	// transaction, such as an unreachable peer. Retried; never dead-lettered or
	// propagated to waiting transactions.
	errDependencyTimeout = errors.New("a dependency could not be resolved")

	// errProducerFailed marks a transaction abandoned without validation because
	// a previous transaction it was waiting on (directly or transitively) was
	// found invalid.
	errProducerFailed = errors.New("a producer of this transaction failed validation")
)

// classifiedError attaches a failure class beside an error rather than wrapping
// it into the message, so Error() stays byte-identical to the cause (tooling may
// match on "failed to validate transaction") while errors.Is still finds the
// class.
type classifiedError struct {
	class error
	cause error
}

func (e *classifiedError) Error() string { return e.cause.Error() }

// Unwrap returns both branches so errors.Is matches the class and the cause.
func (e *classifiedError) Unwrap() []error { return []error{e.class, e.cause} }

// classify tags cause with a failure class. A nil cause stays nil, so it can be
// applied directly to a call's result.
func classify(class, cause error) error {
	if cause == nil {
		return nil
	}
	return &classifiedError{class: class, cause: cause}
}

// stripClass removes the outermost failure class from err, returning its cause.
// Re-tagging must go through this, or the old class would still match errors.Is.
// Classes deeper in the chain are kept on purpose: they are still true.
func stripClass(err error) error {
	var classified *classifiedError
	if errors.As(err, &classified) {
		return classified.cause
	}
	return err
}

// classifyValidationFailure returns errDependencyTimeout if a peer call inside
// validation tagged the error as transient, and errValidationFailed (a verdict)
// otherwise.
func classifyValidationFailure(err error) error {
	if errors.Is(err, errDependencyTimeout) {
		return errDependencyTimeout
	}
	return errValidationFailed
}
