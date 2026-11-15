# Scan implementation verification — 2026-11-15

All 13 implementation tasks are complete on `feat/scan`. The CLI supports 21 AWS resource types, three report formats, usage verdicts, actual/estimated costs, configuration findings and independent controls for paid analysis (`--cost` and `--no-metrics`). Original per-service commands are replaced by `scan --type`.

## Verification

- `go build -o /tmp/lookr .` — passed.
- `go vet ./...` — passed.
- `go test -race ./...` — passed across all packages; no races.
- Executable smoke checks: help includes the nine flags; invalid type, days, output and cost exit 1 with no report in stdout.
- IAM policy JSON parsed and compared against direct and paginated API operations: 52 distinct detected read operations covered, no write API calls.
- No signed/live AWS scan was run; account permission and billing outcomes remain unverified.
- GoReleaser is not installed, so `goreleaser check` and packaging were not run. Config migrated to v2 plural archive formats; release workflow updated. No push, tag, release or merge performed.

## Final review corrections

Independent whole-branch review found seven important issues; each was reproduced before correction and covered by regression tests:

1. Preserve completed regional pages and each ELB family despite later API errors; return resources with warnings.
2. Missing optional security data remains absent. IMDSv1 findings require an observed enabled endpoint and optional-token setting.
3. EBS Multi-Attach is idle only if every attachment is known stopped, independent of order; missing state remains unknown.
4. ElastiCache inventory requests node IDs, CloudWatch queries both node and cluster dimensions, sums across nodes and preserves missing-node uncertainty.
5. Stopped RDS estimates omit compute, retain applicable storage and describe automatic restart after up to seven days.
6. Redirect/fixed-response ALBs with no targets are not automatically orphan; positive action counts prove use and zero observed activity can be idle.
7. `--no-metrics` independently disables paid CloudWatch calls while preserving inventory, structural verdicts and local checks.

The whole-branch review was independent. The final correction pass and regression validation were completed by the controller after fix agents reached the account usage limit; no additional independent scoped review was available for that final pass.

## Recorded decisions

- | 13 vs setup | Task 13 adds .DS_Store to .gitignore | Ruling: done at setup so `git add -A` in tasks never commits .DS_Store — plan intent — no cost |
- Ruling: download SDK dependencies together before dependent tasks — avoid repeated network approvals, compatible with Go 1.25 — cost if wrong: unused requirements removed by final go mod tidy.
- Ruling: Cost Explorer request uses required SERVICE=Amazon Elastic Compute Cloud - Compute, actual limited to EC2; other services estimated — official API requirement (aws-reference-notes.md) — cost: fewer services with actual cost.
- Ruling: explicit positive partial activity proves in_use, missing zero-activity metrics yields unknown — conservative data interpretation — cost if wrong: additional unknown verdicts.
- Ruling: controller implements Task 10 while sole collector implementer handles Task 5 — independent pure model consumers; reduces idle time without shared-file edits — cost if wrong: task order only.
- Ruling: CloudWatch PartialData followed by Complete is valid pagination; terminal partial/missing/error results unknown — official MetricDataResult API — cost if wrong: conservative unknown instead of false idle.
- Ruling: security checks require observed config keys rather than treating missing values as disabled controls — missing permissions must not invent findings — cost if wrong: absent controls may go unreported when collection incomplete.
- Ruling: controller advances Task 9 independently of collectors while waiting for git access — model-only API, exact plan interface — cost if wrong: task order only.
- Ruling: CLI captures live progress to avoid duplicate final warning lines — required fake provider warning behavior plus live warnings — cost: small memory buffer proportional to region/service warnings.
- Ruling: go install . from checkout because required module path stays lookr — remote module path install may fail — cost: requires clone before install.
- Ruling: preserve partial inventories, unknown security field presence, and every Multi-Attach state — prevents false findings/waste — cost if wrong: more unknown outcomes.
- Ruling: ElastiCache uses actual node IDs with two dimensions and aggregate sums only if every node observed — AWS publishes node metrics — cost if wrong: more queries.
- Ruling: ALB fixed responses and redirects prove use; targetless inactive ALBs are idle, never automatically orphan — valid ALB actions — cost if wrong: fewer orphan classifications.
- Ruling: stopped RDS estimate omits compute and notes automatic restart up to 7 days — correct current stopped run rate — cost if wrong: eventual restart makes future cost higher than estimate.
- Ruling: add --no-metrics independent of --cost — spec required both paid paths switchable though plan omitted flag — cost if wrong: ninth CLI flag/API field.
- Ruling: final corrections use root after two fix agents hit account usage limit — no remaining agent capacity — cost: scoped fix verification by author instead of another reviewer.

## Nonblocking review backlog

These items do not represent unfinished implementation-plan tasks. They are test-coverage, formatting or documented estimator limitations identified during review.

- Task 1: minor (deferred): empty/unrecognized Verdict sorts as orphan and adds "" key to ByVerdict (report.go)
- Task 1: minor (deferred): no test asserts 2-decimal rounding in summary
- Task 1: minor (deferred): sort comparator copies Resource structs; Strs ignores []any
- Task 2: minor (deferred): money() panics on NaN/Inf
- Task 2: minor (deferred): finding message/recommendation not newline-sanitised in table
- Task 2: minor (deferred): no unit tests for money/truncate/plural (thousands separator, 40-char cut never exercised)
- Task 2: minor (deferred): TestWriteTableEmpty does not assert absence of TYPE header / FINDINGS block
- Task 2: minor (deferred): empty-report branch write-error path not covered by test
- Task 3: minor (deferred): exact namespace/metric/stat query assertions could be broader; Aurora stopped/missing/zero test branches share tested RDS evaluator.
- Task 9: minor (deferred): rare nil/currency/malformed/repeated-token error branches lack dedicated regression cases.
- Task 5: minor (deferred): SG native GroupName fallback uses GroupId.
- Task 6: minor (deferred): some fake pagination tests do not assert incoming token.
- Task 7: complete (ae6ef3e + d35cea5, scope fix approved); minor (deferred): explicit nonnil certificate RFC3339 timestamp assertion.
- Task 8: complete (cc38fa5 + 5efbcfe, scope fix approved); minor (deferred): global pricing cache lock delays canceled concurrent callers (current scan estimates sequentially).
- Task 11: complete (020dce1, full race suite pass; independent review approved); minor (deferred): direct missing-actual EC2 fallback assertion.
- Final: minor (deferred): CLI cancellation/context-forwarding direct unit coverage (orchestrator cancel tested).

The final review deliberately retains the approved on-demand/lowest-positive-price assumptions rather than modeling discounts, egress or exact billing tiers. See README cost notes. Live AWS and packaging validation can be performed separately before publishing a release.
