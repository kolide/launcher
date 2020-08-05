package service

import (
	"github.com/go-kit/kit/log"
)

type Middleware func(KolideService) KolideService

func LoggingMiddleware(logger log.Logger) Middleware {
	return func(next KolideService) KolideService {
		return logmw{logger, next}
	}
}

type logmw struct {
	logger log.Logger
	next   KolideService
}

func uuidMiddleware(next KolideService) KolideService {
	return uuidmw{next}
}

type uuidmw struct {
	next KolideService
}
