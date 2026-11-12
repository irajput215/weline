// Package audiences builds broadcast lists from the organization's contacts:
// saved segments, turning a segment or a retarget of a past campaign into a
// draft campaign in Weline's Campaigns, and a history of what was created.
// Sending, scheduling and delivery stay in upstream Campaigns.
//
// Spec: wl-docs/modules/audiences/. Weline-only package.
package audiences

// Name is the module's short name: routes /api/x/audiences, tables
// wl.audiences_*, permissions wl_audiences_*.
const Name = "audiences"
