package main

import "errors"

// Sentinel errors used across the gateway.
var (
	errInvalidToken = errors.New("invalid or unauthorized token")
)
