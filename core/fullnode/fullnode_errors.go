package fullnode

import "errors"

// Failure classification for the fullnode transaction pipeline.
//
// Two questions are asked of every failure, and until now neither could be
// answered: is this transaction wrong, or did something around it go wrong? The
// pipeline treated both the same — three attempts, then an audit row selected by
// a substring match on the error text.
//
// The distinction matters in two places. A transaction this node has decided is
// invalid will be just as invalid on the next attempt, and everything waiting to
// build on it is waiting for something that will never arrive. A peer that could
// not be reached says nothing about the transaction at all, and retrying is the
// entire recovery mechanism for it.
//
// Getting this wrong in the permissive direction is cheap: an unclassified
// failure falls back to retrying, which is what the pipeline did for everything
// before. Getting it wrong in the other direction is not — a transient failure
// mistaken for a verdict would dead-letter a good transaction and take
// everything downstream of it as well. So only failures this node reached on its
// own evidence are deterministic, and anything that went through a peer is
// transient by construction.

var (
	// errValidationFailed marks a transaction this node has judged invalid on
	// its own evidence: a signature that does not verify, a chain that does not
	// join up, an ownership claim that does not hold. Deterministic — the next
	// attempt reads the same data and reaches the same verdict.
	errValidationFailed = errors.New("transaction failed validation")

	// errDependencyTimeout marks a failure that says nothing about the
	// transaction: a peer that could not be reached, a chain that could not be
	// fetched. Transient — the retry ladder is what recovers from these, and
	// they must never be dead-lettered or propagated.
	errDependencyTimeout = errors.New("a dependency could not be resolved")

	// errProducerFailed marks a transaction abandoned without being validated,
	// because a transaction it declares as a producer was found invalid. It
	// cannot succeed: it claims to spend an output of something that this node
	// has determined never legitimately existed.
	errProducerFailed = errors.New("a producer of this transaction failed validation")
)

// classifiedError attaches a failure class to an error without changing what it
// says.
//
// The message is load-bearing. The audit trail used to be selected by
// strings.Contains on "failed to validate transaction", and log tooling outside
// this repository may still match on it, so the switch to typed errors must not
// alter a single byte of any message. Wrapping with fmt.Errorf would splice the
// class's own text in, so the class is attached as a second branch of the error
// tree instead: errors.Is finds it, and Error() returns exactly what it wrapped.
type classifiedError struct {
	class error
	cause error
}

func (e *classifiedError) Error() string { return e.cause.Error() }

// Unwrap returns both branches, which is what lets errors.Is match the class and
// the original cause alike.
func (e *classifiedError) Unwrap() []error { return []error{e.class, e.cause} }

// classify tags cause with a failure class. A nil cause stays nil, so it can be
// applied directly to a call's result.
func classify(class, cause error) error {
	if cause == nil {
		return nil
	}
	return &classifiedError{class: class, cause: cause}
}

// classifyValidationFailure decides whether a failure reported by validation is
// this node's verdict on the transaction or a report about its surroundings.
//
// Validation performs network calls — it fetches chains and genesis entries from
// peers to fill gaps — so an unreachable peer surfaces here as a validation
// failure even though nothing about the transaction is in question. Those calls
// tag their errors as they return, and finding such a tag anywhere in the chain
// is what makes this transient. Everything else is a verdict.
func classifyValidationFailure(err error) error {
	if errors.Is(err, errDependencyTimeout) {
		return errDependencyTimeout
	}
	return errValidationFailed
}
