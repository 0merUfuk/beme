package main

import "time"

func cmdNowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
