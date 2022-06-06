package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
)

type httpLogger struct {
	logr.Logger
}

func (l httpLogger) Print(v ...interface{}) {
	l.Info(fmt.Sprint(v...))
}

func serverLogger(logr logr.Logger) func(http.Handler) http.Handler {
	formatter := &middleware.DefaultLogFormatter{
		Logger: httpLogger{logr},
	}
	return middleware.RequestLogger(formatter)
}
