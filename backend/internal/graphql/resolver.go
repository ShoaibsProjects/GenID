package graphql

import (
	"github.com/observeid/genid/internal/services"
)

type Resolver struct {
	Svc *services.Service
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}

func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}
