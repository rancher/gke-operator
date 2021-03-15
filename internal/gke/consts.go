package gke

// Status indicates how to handle the response from a request to update a resource
type Status int

// Status indicators
const (
	// Changed means the request to change resource was accepted and change is in progress
	Changed Status = iota
	// Retry means the request to change resource was rejected due to an expected error and should be retried later
	Retry
	// NotChanged means the resource was not changed, either due to error or because it was unnecessary
	NotChanged
)

// Error strings from the provider
const (
	errNotFound = "notFound"
	errWait     = "Please wait and try again once it is done"
)
