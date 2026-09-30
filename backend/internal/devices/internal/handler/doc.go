// Package handler adapts HTTP requests for the devices module to its service. Handlers decode and
// validate input, call the service and map results to the generated response types. They never
// touch the database or another module's internals.
package handler
