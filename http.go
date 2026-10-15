package main

import (
	"net/http"
	"time"
)

func newHTTPClient() *http.Client { return &http.Client{Timeout: 45 * time.Second} }
