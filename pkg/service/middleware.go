package service

import (
	"github.com/go-kit/kit/log"
)

func LoggingMiddleware(logger log.Logger) func(KolideService) KolideService {
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
