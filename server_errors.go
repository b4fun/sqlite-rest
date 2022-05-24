package main

import (
	"fmt"
	"net/http"
)

type ServerError struct {
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
	Hint       string `json:"hint,omitempty"`
	StatusCode int    `json:"-"`
}

func (se *ServerError) Error() string {
	if se.Hint != "" {
		return fmt.Sprintf("%s - %s", se.Message, se.Hint)
	}
	return se.Message
}

func (se *ServerError) WithHint(hint string) *ServerError {
	rv := new(ServerError)
	*rv = *se
	rv.Hint = hint
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
