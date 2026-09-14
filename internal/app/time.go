package app

import "time"

func nowUTC() string        { return time.Now().UTC().Format(time.RFC3339) }
func timeNowUTC() time.Time { return time.Now().UTC() }
