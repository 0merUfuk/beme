package app

import "errors"

// ErrTransportUnsupported: v1 serves MCP over stdio only (ADR-014); no network
// listener exists, so no unauthenticated network server can start.
var ErrTransportUnsupported = errors.New("only stdio transport is supported in v1 (ADR-014)")

// ValidateTransport accepts exactly "stdio".
func ValidateTransport(transport string) error {
	if transport == "stdio" {
		return nil
	}
	return ErrTransportUnsupported
}
