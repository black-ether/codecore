// Package sample exercises codecore's Go extractor: every symbol kind,
// generics, embedded fields, grouped declarations, and traps — a string
// and a comment that look like code but must never appear in the outline.
package sample

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

import "strings"

const (
	// DefaultPort is the port the server listens on.
	DefaultPort = 8080
	MaxRetries  = 3
)

var (
	// DefaultTimeout applies to outbound calls.
	DefaultTimeout = 30 * time.Second
	version        = "dev"
)

// Handler handles one HTTP request.
type Handler func(w http.ResponseWriter, r *http.Request)

// Alias is an alias for int.
type Alias = int

// Config holds server settings.
type Config struct {
	Port        int    `json:"port"`
	Host        string `json:"host"`
	http.Server        // embedded
	Addr, Name  string
}

// Pair holds two values of the same type.
type Pair[T any] struct {
	First, Second T
}

// Greeter greets people.
type Greeter interface {
	Greet(name string) string
	io.Reader // embedded
}

// Server serves HTTP.
type Server struct {
	cfg Config
}

// NewServer builds a Server.
func NewServer(cfg Config) *Server { return &Server{cfg: cfg} }

// Map applies f to every element.
func Map[T any, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// Start starts the server.
func (s *Server) Start() error { return nil }

// Addr returns the configured host.
func (s Server) Addr() string { return s.cfg.Host }

func main() {
	fmt.Println("hi")
	_ = strings.TrimSpace("x")
	trap := "func fake() {}"
	_ = trap
	// func commentedOut() {}
	anon := func() {}
	_ = anon
}
