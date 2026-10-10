package engine

import "gw-base/internal/config"

// answer is what an upstream said, as business error rules see it: the HTTP
// status (path "@status") and the JSON body (paths "$.a.b"), when there is one.
type answer struct {
	status int
	doc    any // nil when the body is not JSON
}

// value resolves a rule path; ok is false when it is not in the answer.
func (a answer) value(path string) (any, bool) {
	if path == config.PathStatus {
		return a.status, true
	}
	return lookupPath(a.doc, path)
}
