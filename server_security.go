package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/pflag"
)

// TODO: generally speaking, we need a fine-grained RBAC system.

type ServerSecurityOptions struct {
	// EnabledTableOrViews list of table or view names that are accessible (read & write).
	EnabledTableOrViews []string
}

func (opts *ServerSecurityOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(
		&opts.EnabledTableOrViews,
		"security-allow-table",
		[]string{},
		"list of table or view names that are accessible (read & write)",
	)
}

func (opts *ServerSecurityOptions) defaults() error {
	return nil
}

func (opts *ServerSecurityOptions) createTableOrViewAccessCheckMiddleware(
	responseErr func(w http.ResponseWriter, err error),
) func(http.Handler) http.Handler {
	accessibleTableOrViews := make(map[string]struct{})
	for _, t := range opts.EnabledTableOrViews {
		accessibleTableOrViews[t] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			target := chi.URLParam(req, routeVarTableOrView)

			if _, ok := accessibleTableOrViews[target]; !ok {
				responseErr(w, ErrAccessRestricted)
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}
