package graphql

import (
	"github.com/observeid/identity-platform/internal/service"
)

type Resolver struct {
	Svc *service.IdentityService
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}

func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}
