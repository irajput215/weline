# Industry packs: test plan

Go (`internal/modules/packs/packs_test.go`, real Postgres):

| Case | Test |
| --- | --- |
| Every pack file loads and validates; vertical → pack; every onboarding vertical has a pack | `TestPackFilesAreValid` |
| Bad packs refused: id, prefix, case, category, marketing without STOP, variable order, samples, buttons, unknown template, bad rules | `TestValidateRefusesBadPacks` |
| Install without WhatsApp (templates wait), AI applied but not turned on, segments created, rules wait; with a number templates become drafts with buttons and samples; again → exists; after approval rules created off; audited; unknown pack 404 | `TestInstall_ClinicPack` |
| Your own AI instructions and tone kept; emergency words merged without duplicates | `TestInstall_KeepsYourOwnAISetup` |
| Recommended first for the onboarding vertical; another organization sees no installs | `TestList_RecommendedAndIsolated` |
| Routes refuse without permission; browse vs install; 404 | `TestHandlers` |
| Real estate pack installs with nothing failing (segments valid), AI guardrails, recommended for real estate, inspection reminder uses property and time | `TestInstall_RealEstatePack` |

Frontend: `stores/packs.spec.ts`. Journey (`e2e/packs.e2e.ts`): preview and install the clinic pack, install again
changes nothing; reception has no page.
