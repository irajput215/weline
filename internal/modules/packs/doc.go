// Package packs is the Weline module for vertical packs (build guide 7.1).
//
// Spec: wl-docs/modules/packs/. A pack is data, not code (data/*.json): an
// industry's WhatsApp templates (created as drafts to submit to Meta), AI
// agent instructions and emergency words, suggested audience segments,
// reminder and recall rules, chatbot flow guides, and which connectors fit.
// Installing is idempotent and can be run again: reminder rules wait until
// their template is approved by Meta, so a second install creates them.
package packs
