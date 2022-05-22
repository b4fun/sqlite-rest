package main

import (
	"fmt"
	"net/http"
)

type ServerError struct {
	Message    string `json:"message"`
	Hints      string `json:"hints,omitempty"`
	StatusCode int    `json:"-"`
}

func (se *ServerError) Error() string {
	if se.Hints != "" {
		return fmt.Sprintf("%s - %s", se.Message, se.Hints)
	}
	return se.Message
}

func (se *ServerError) WithHints(hints string) *ServerError {
	rv := new(ServerError)
	*rv = *se
	rv.Hints = hints
	return rv
}

var (
	ErrUnsupportedMediaType = &ServerError{
		Message:    "Unsupported Media Type",
		StatusCode: http.StatusUnsupportedMediaType,
	}

	ErrBadRequest = &ServerError{
		Message:    "Bad Request",
		StatusCode: http.StatusBadRequest,
	}
)
