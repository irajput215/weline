// Package operator is the Weline-staff console across organizations (build
// guide 6.3; ADR-005).
//
// Spec: wl-docs/modules/operator/. Only upstream super admins get in (on top
// of the wl_operator_console permission, which no role is given). It shows
// sign-ups, plan, usage, connector health and error counts per organization,
// never patients' details or messages, and every view is audited: in the
// operator's own organization, and in the organization looked at, so its
// admins can see when Weline staff looked.
package operator
