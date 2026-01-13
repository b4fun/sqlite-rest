package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/pflag"
)

const (
	headerNameAuthorizer = "Authorization"
	headerPrefixBearer   = "Bearer"
)

type ServerAuthOptions struct {
	RSAPublicKeyFilePath string
	TokenFilePath        string

	// for unit test
	disableAuth bool
}

func (opts *ServerAuthOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.RSAPublicKeyFilePath, "auth-rsa-public-key", "", "path to the RSA public key file")
	fs.StringVar(&opts.TokenFilePath, "auth-token-file", "", "path to the token file")
}

func (opts *ServerAuthOptions) defaults() error {
	if opts.disableAuth {
		return nil
	}

	if opts.RSAPublicKeyFilePath == "" && opts.TokenFilePath == "" {
		return fmt.Errorf("specifies at least --auth-rsa-public-key or --auth-token-file")
	}

	if opts.RSAPublicKeyFilePath != "" && opts.TokenFilePath != "" {
		return fmt.Errorf("cannot specific --auth-rsa-public-key and --auth-token-file at the same time")
	}

	return nil
}

func (opts *ServerAuthOptions) createAuthMiddleware(
	responseErr func(w http.ResponseWriter, err error),
) func(http.Handler) http.Handler {
	if opts.disableAuth {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	var validMethods []string
	jwtKeyFunc := jwt.Keyfunc(func(t *jwt.Token) (interface{}, error) {
		return nil, fmt.Errorf("invalid token")
	})

	// NOTE: we re-read token from disk to allow reloading public keys

	switch {
	case opts.RSAPublicKeyFilePath != "":
		keyReader := readFileWithStatCache(opts.RSAPublicKeyFilePath)

		validMethods = []string{
			jwt.SigningMethodRS256.Name,
			jwt.SigningMethodRS384.Name,
			jwt.SigningMethodRS512.Name,
		}
		jwtKeyFunc = func(t *jwt.Token) (interface{}, error) {
			b, err := keyReader()
			if err != nil {
				return nil, err
			}

			v, err := jwt.ParseRSAPublicKeyFromPEM(b)
			if err != nil {
				return nil, err
			}
			return v, nil
		}
	case opts.TokenFilePath != "":
		tokenReader := readFileWithStatCache(opts.TokenFilePath)

		validMethods = []string{
			jwt.SigningMethodHS256.Name,
			jwt.SigningMethodHS384.Name,
			jwt.SigningMethodHS512.Name,
		}
		jwtKeyFunc = func(t *jwt.Token) (interface{}, error) {
			b, err := tokenReader()
			if err != nil {
				return nil, err
			}

			return b, nil
		}
	}

	jwtParser := jwt.NewParser(jwt.WithValidMethods(validMethods))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := r.Header.Get(headerNameAuthorizer)

			if v == "" {
				responseErr(w, ErrUnauthorized.WithHint("missing auth header"))
				return
			}

			ps := strings.SplitN(v, " ", 2)
			if len(ps) != 2 {
				responseErr(w, ErrUnauthorized.WithHint("invalid auth header"))
				return
			}

			if !strings.EqualFold(ps[0], headerPrefixBearer) {
				responseErr(w, ErrUnauthorized.WithHint("invalid auth header"))
				return
			}

			// TODO: add rbac support
			_, err := jwtParser.Parse(ps[1], jwtKeyFunc)
			if err != nil {
				responseErr(w, ErrUnauthorized.WithHint(err.Error()))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
