// Package kit holds the shared building blocks for Weline modules: the module
// contract, the migration runner, the permission seeder, request helpers,
// typed errors and the WhatsApp sender.
//
// Weline-only package. Nothing here is part of upstream whatomate, so it never
// conflicts when upstream is merged. See wl-docs/sdlc/weline-module-sdlc.md.
package kit

import (
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// Schema is the Postgres schema that holds every module table.
const Schema = "wl"

// Module is the contract every Weline module implements. The registry
// (internal/modules/registry.go) calls these from the two Go seams.
type Module interface {
	// Name is the module's short name, e.g. "booking". It prefixes the
	// module's migration IDs, tables (wl.<name>_*), routes (/api/x/<name>)
	// and permissions (wl_<name>_*).
	Name() string
	// Register mounts the module's routes. Called from setupRoutes.
	Register(g *fastglue.Fastglue)
	// Migrations lists the module's versioned migrations, oldest first.
	Migrations() []Migration
	// Permissions lists the permissions the module checks.
	Permissions() []Permission
}

// Migration is one versioned, reversible schema change for a module.
//
// ID must be unique within the module and never change once released, for
// example "0001_create_notes". The runner stores it as "<module>/<ID>" in
// wl.schema_migrations so modules can't clash.
type Migration struct {
	ID   string
	Up   func(tx *gorm.DB) error
	Down func(tx *gorm.DB) error
}

// Permission is a resource/action pair a module checks, e.g.
// {Resource: "wl_booking_appointments", Action: "read"}.
type Permission struct {
	Resource    string
	Action      string
	Description string
	// Roles names the system roles ("admin", "manager", "agent") that get this
	// permission when it is first created. Later runs never re-grant it, so an
	// admin who removes it from a role keeps that choice.
	Roles []string
}

// Actions modules may use, matching the SDLC doc.
const (
	ActionRead   = "read"
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)
