package main

import (
	"context"
	"time"
)

type model struct {
	websites          []string
	schedule          string
	timezone          *time.Location
	healthEndpoint    []string
	lastPing          []string
	lastError         []string
	lastHealthGeneric []map[string]any
	quit              bool
	ctx               context.Context
	cancel            context.CancelFunc
}

func initialModel(websites []string, schedule string, healthEndpoints []string, ctx context.Context, cancel context.CancelFunc) model {
	return model{
		websites:          websites,
		schedule:          schedule,
		timezone:          time.Local,
		healthEndpoint:    healthEndpoints,
		lastPing:          make([]string, len(websites)),
		lastError:         make([]string, len(websites)),
		lastHealthGeneric: make([]map[string]any, len(websites)),
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
