package broker

import "github.com/michaelquigley/terminus/internal/findings"

const (
	VerdictClean    = "clean"
	VerdictNotClean = "not_clean"
)

func Clean(classified []findings.Classified) (bool, []findings.Finding) {
	var blocking []findings.Finding
	for _, c := range classified {
		if c.Blocking {
			blocking = append(blocking, c.Finding)
		}
	}
	return len(blocking) == 0, blocking
}
