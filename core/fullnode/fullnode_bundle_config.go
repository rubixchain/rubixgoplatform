package fullnode

import "time"

// bundleConfig holds the settings for the dependency-aware ingest path, which is
// always on. A waiting transaction occupies a worker, so waits are short, and
// every expiry falls through to normal validate-and-sync.
type bundleConfig struct {
	// inflightWait is how long to wait for a previous transaction this node
	// holds (queued or in flight); it is expected to be persisted soon.
	inflightWait time.Duration

	// unknownWait is how long to wait for a previous transaction this node does
	// not hold. Much shorter, because a fullnode that joined late may never see
	// it, and a long wait would delay every such token.
	unknownWait time.Duration

	// syncMemoTTL is how long a successful chain sync is remembered, so a later
	// transaction in the same bundle does not repeat it. Short because a wrong
	// skip can turn into a verdict (see fullnode_syncmemo.go).
	syncMemoTTL time.Duration
}

func defaultBundleConfig() bundleConfig {
	return bundleConfig{
		inflightWait: 5 * time.Second,
		unknownWait:  1 * time.Second,
		syncMemoTTL:  5 * time.Second,
	}
}
