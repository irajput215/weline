// Package connectors connects Weline to the system a business already uses
// for its diary (Cliniko first; ADR-002). It owns the per-organization choice
// of provider and its encrypted credentials (wl.connector_accounts), links
// between Weline records and the provider's (wl.connector_links), the sync log
// (wl.connector_syncs), the Settings → Connected system screen, and the
// resolver every caller books through (package provider).
// Spec: wl-docs/modules/connectors.
package connectors

// Name is the module name used for migrations and logs.
const Name = "connectors"
