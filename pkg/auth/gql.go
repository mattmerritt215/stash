package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/stashapp/stash/pkg/logger"
)

// GQLGate returns a gqlgen operation middleware that rejects any operation
// whose root fields the caller is not permitted to execute.
//
// This is the enforcement point. The frontend also hides controls the user
// cannot use, but that is cosmetic — this is what actually stops a hand-rolled
// GraphQL request from reconfiguring the server.
//
// Unknown root fields are denied unless the caller is an admin, so a mutation
// added by a future upstream release is locked down until it is classified in
// roles.go.
func GQLGate() graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		u := FromContext(ctx)
		if u == nil {
			// no identity attached: either the multi-user layer is disabled
			// or this is an internal call (plugin hook, DLNA, CLI).
			return next(ctx)
		}

		opCtx := graphql.GetOperationContext(ctx)
		if opCtx == nil || opCtx.Operation == nil {
			return next(ctx)
		}

		opType := string(opCtx.Operation.Operation)
		for _, sel := range opCtx.Operation.SelectionSet {
			field, ok := sel.(*ast.Field)
			if !ok {
				continue
			}
			name := field.Name
			if strings.HasPrefix(name, "__") {
				// introspection
				continue
			}

			perm := PermissionForOperation(opType, name)
			if u.Can(perm) {
				continue
			}

			logger.Warnf("auth: denied %s.%s to %s (role %s, needs %s)",
				opType, name, u.Username, u.Role, perm)

			return graphql.OneShot(graphql.ErrorResponse(ctx,
				"%s", fmt.Sprintf("not permitted: %s requires the %q permission", name, perm)))
		}

		return next(ctx)
	}
}
