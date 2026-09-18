package routing

import "time"

type ServiceOption func(*Service)

// WithClock supplies the evaluation clock. Configure it when constructing the
// service so all decisions in a calculation share the same reference time.
func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}
