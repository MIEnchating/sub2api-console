package adminclient

import "context"

type mutationAuthorizationKey struct{}

// MutationAuthorizationError distinguishes an unsent write from a retry that
// was stopped after an earlier request may already have reached the server.
type MutationAuthorizationError struct {
	Cause     error
	Attempted bool
}

func (err *MutationAuthorizationError) Error() string { return err.Cause.Error() }
func (err *MutationAuthorizationError) Unwrap() error { return err.Cause }

func WithMutationAuthorization(ctx context.Context, authorize func(context.Context) error) context.Context {
	return context.WithValue(ctx, mutationAuthorizationKey{}, authorize)
}

func AuthorizeMutation(ctx context.Context) error {
	authorize, _ := ctx.Value(mutationAuthorizationKey{}).(func(context.Context) error)
	if authorize == nil {
		return nil
	}
	if err := authorize(ctx); err != nil {
		return &MutationAuthorizationError{Cause: err}
	}
	return nil
}
