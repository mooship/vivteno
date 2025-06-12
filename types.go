package main

import (
	"context"
	"time"
)

type model struct {
	website           string
	schedule          string
	timezone          *time.Location
	healthEndpoint    string
	lastPing          string
	lastError         string
	lastHealthGeneric map[string]any
	quit              bool
	ctx               context.Context
	cancel            context.CancelFunc
}

func initialModel(website, schedule string, ctx context.Context, cancel context.CancelFunc) model {
	return model{
		website:           website,
		schedule:          schedule,
		timezone:          time.Local,
		healthEndpoint:    "",
		lastPing:          "",
		lastError:         "",
		lastHealthGeneric: nil,
		quit:              false,
		ctx:               ctx,
		cancel:            cancel,
	}
}

type tickMsg time.Time

type pingResult struct {
	result string
	err    error
}

type healthResultGeneric struct {
	data map[string]any
	err  error
}
