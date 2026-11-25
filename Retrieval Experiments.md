# Codecore experimental retrieval

On the fixed natural top-50 queries, identifier splitting and production filtering raised selected-anchor hits from **4/200 to 23/200**. Adding grouping returned **14/200** with less repetition and surfaced a checksum error span at rank 48. All four exact-identifier controls, including the two held-out controls, kept their target statement at rank 1 in every profile.

## Fixed and held-out results

The four fixed natural queries originally returned 20 selected hits in the 179-file subset and 4 in the complete checkout. The table compares all new profiles on the **same complete checkout**. Each profile returned 200 results across those four queries. Unit counts sum the per-query distinct counts. Duration is the median of the four per-query medians.

| Profile | Selected | Direct | Span | Distinct units | Distinct Go functions | Stdout bytes | Median ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Original | 4 | 4 | 0 | 191 | 30 | 65,355 | 496.1 |
| Identifiers | 8 | 8 | 0 | 183 | 36 | 65,046 | 469.4 |
| Production only | 20 | 20 | 0 | 172 | 73 | 77,745 | 270.2 |
| Identifiers + production | 23 | 23 | 0 | 159 | 79 | 77,143 | 267.0 |
| Grouping only | 3 | 3 | 0 | 200 | 33 | 65,388 | 513.3 |
| Identifiers + production + grouping | 14 | 11 | 4 | 200 | 102 | 76,599 | 278.4 |
| Duplicate suppression only | 5 | 5 | 0 | 191 | 33 | 65,623 | 508.9 |
| All four options with production only | 14 | 11 | 4 | 200 | 102 | 76,599 | 268.0 |

**Identifier splitting.** N2 added four direct merge hits at ranks 19, 20, 21 and 24. S1 gained a span over the leading-colon branch at rank 31. N1 still missed the selected anchors, and S3 still missed the sort line. The two fixed identifier controls retained their direct rank 1 hits and their original enclosing spans at ranks 3 and 6.

**Source categories.** Production filtering alone recovered three N1 hits in include or merge code and raised N2 from 2 to 13 selected results. Median fixed-natural duration fell from 496.1 ms to 270.2 ms. Filtering preferentially favors the benchmark’s implementation anchors and can hide docs or tests relevant to exploratory queries. It recovered no held-out natural hits on its own.

**Grouping.** For the identifiers-plus-production pair, grouping raised distinct Go functions and methods from 79 to 102 across the fixed natural lists. It reduced repeated direct hits, including N2’s cluster within the merge implementation. N2 gained a lookup span at rank 37. The representative statement was at task.go:604 and the enclosing GetTask range was 580–605. N3 gained `IsUpToDate` at rank 48, with its strongest line as evidence and an outline range covering five selected freshness error anchors. The natural queries still returned zero direct hits on the original four missed details. S4 kept the error span at rank 1 and lost the additional direct output-error and write-error lines selected by the corresponding ungrouped profile. Imports remained searchable. The two fixed identifier lists contained 21 import results originally and 19 with identifiers, production filtering and grouping.

**Duplicate suppression.** With full coverage, N1’s two identical local-includes passages collapsed to one representative, and a merge line entered at rank 44. Across the four natural lists, the repeated Markdown line diagnostic fell from 36 to 26. Total stdout increased by 268 bytes as other results took the freed slots. The all-options production-only results were identical to the preceding production-only profile because that category filter excluded Markdown. A separate paired comparison below keeps documentation eligible.

**Held-out checks.** The signal interruption natural query missed its anchors across all standard profiles. The buffered-output natural query went from a miss to a direct hit at rank 23 with identifier splitting and rank 3 with identifiers and production filtering, which grouping preserved. The informed buffered-output query gained a direct hit at rank 9 with identifiers and rank 7 with the three combined retrieval options. For the informed signal query, grouping surfaced the signal-registration line through a span at rank 2 in the combined profile. The held-out identifier queries retained their rank 1 registration and output-buffer statements.

### Duplicate suppression with documentation retained

Both profiles enable identifiers and grouping and select `production,docs`. Adding duplicate suppression reduced the repeat diagnostic from 56 to 41 across the fixed natural lists and raised selected direct hits from 4 to 5. N1 gained the same merge-loop line at rank 44. Fixed-natural stdout increased from 63,819 to 64,158 bytes and median duration went from 423.7 to 428.9 ms. The held-out natural pair stayed at one hit while the repeat diagnostic fell from 17 to 11.

### Corpus inventory and token counts

The original walk discovered 660 eligible files and produced 52,958 searchable outline nodes on the pinned checkout. Stage 2 classified those files as 178 production, 123 documentation, 43 test, 315 fixture and one generated/mock. That inventory helps explain the time difference when filtering before extraction.

The original tokenizer counted 279,286 terms toward document length. Stage 1 counted 276,576 original lexemes, down 0.97%, because underscore-connected names count once. Its expanded postings contained 374,621 unboosted term occurrences. Exact term frequency for `include` rose from 300 to 353, and `lines` went from 53 to 121. The complete identifiers `taskNameWithNamespace` and `collectKeys` stayed at seven and three occurrences. For example, `read_read` contributes one length unit and one `read` alias per occurrence. This caps duplicate components within a single identifier while still counting separate occurrences of that identifier. [Index and category measurements](experiment/data/index-stats.json)

## Reference and method

The reference is the sibling workspace’s Retrieval Quality Investigation.md. The experiment uses the clean, full Task checkout at `b5aef1c287a9be8038347a17646bdf4bc7185bce`. The runner verifies the 982 tracked Git blobs, normalizing Windows symlink targets in the same way as the research preparation script. It reads the canonical queries and anchors and replays the command manifest produced by the research workspace’s `repro/run.py`. The original executable is the research build of `26b8bde`, verified against its recorded SHA-256.

The fixed suite preserves the four natural queries, four implementation-informed queries and two exact-identifier controls. The held-out suite adds two of each query type for Task’s signal handling and buffered output prefixing. Each scope has fixed source file and line anchors. The held-out natural queries were written without the implementation identifiers that appear in the informed and identifier controls. [Held-out queries and anchors](experiment/held-out.json)

Each profile returns up to 50 JSON results. A **direct hit** starts on an anchor line. A **span hit** overlaps an anchor range from a different start, using the enclosing unit’s range for grouped hits. A result can be direct for one anchor and span for another. Selected hits count each output result once. The `anchor_ranks` column records every qualifying 1-based rank by anchor and hit type. `distinct_units` uses Go functions, methods, types and declaration blocks recognized from the original outline, with file and line as the fallback. `distinct_go_functions` counts just the named Go functions and methods. This gives the variants a common unit measure even when they do not emit grouping metadata.

The runner takes one warm-up and five timed subprocess samples per query and profile. It shuffles the order with a fixed seed within each sample round and checks stdout and stderr against the warm-up. Duration includes process startup, file discovery, parsing, retrieval and JSON serialization. Byte counts are captured stdout and stderr before normalization. The Markdown repeat diagnostic counts additional files exposing the same displayed Markdown line of at least 32 characters. The suppression feature independently compares full source paragraphs.

## Implementation and regression coverage

| Stage | Option and behavior | Commit |
| --- | --- | --- |
| 1 | `--split-identifiers` indexes case-folded original lexemes and distinct camelCase, PascalCase, snake_case, acronym and digit parts per occurrence. Query compounds stay whole. Each original occurrence contributes one unit of document length. Only complete definition-name terms receive the name boost and only complete lexical terms receive the exact-word bonus. `--word` disables prefixes and can match an indexed component. | `b3a4a6b` |
| 2 | `--categories production,docs,tests,fixtures,generated` selects any combination before parsing or indexing. File and source-header conventions apply the first matching category in precedence order, starting with fixtures, generated or mock files, tests, documentation and then production. The established file walk and `--no-tests` still apply. | `fe18d0b` |
| 3 | `--group-structure` keeps the strongest ranked statement for each enclosing unit before `--per-file` and `--top`. The result carries the match line and text plus `unit.kind`, `unit.name`, `unit.start` and `unit.end` when a unit is recognized. Its score is the best statement score. | `db0ed04` |
| 4 | `--dedup-docs` retains the best-ranked file for each repeated line offset in an identical, eligible Markdown paragraph. It takes effect after line scoring and before grouping and result limits. | `db06dde` |

The category classifier uses directory and filename conventions and recognized generated-file markers in the first 8 KiB. The eligible production bucket includes configuration files and unfamiliar layouts. HTML extensions are treated as documentation, and matching directory names in an input path can affect classification. The generated bucket includes common mock paths and names. Fixture directories take precedence over those signals. The original walk still determines which files are eligible for automatic discovery.

Grouping uses existing outline ancestry for Go, Python, JavaScript, TypeScript, PowerShell and Rust. It recognizes common named functions and some language-specific declaration units, with ordinary line output as a fallback. Go method names include their receiver. Scope coverage follows the outline and ends at its last retained line. Closing delimiters omitted from the outline may also be absent from the displayed range. Matching lines inside anonymous functions can share the enclosing named unit. Repetition can still affect corpus term statistics. It adds no votes to the grouped rank.

Duplicate suppression compares original paragraph text and line offset across Markdown files. It leaves headings, prose blocks shorter than 32 bytes, fenced and indented examples, and other recognized markup in the result pool. It preserves matches from every source implementation and repeated passages within a single preferred file. Different Markdown paragraphs sharing a displayed line remain eligible, as do duplicated schema or installer content. Matching relative Markdown links can still resolve in different document contexts. Disabling this option exposes all copies.

The original flags reproduced byte-identical stdout and stderr for ten canonical top-50 commands on each of the first three executables and all 94 canonical Codecore commands on Stage 4. CLI option checks covered every category, the existing no-tests behavior, incompatible count options, identifier components, text grouping and source preservation. `--group-structure` and `--dedup-docs` reject `--count`, which reports matching outline lines per file. The Go regressions cover identifier boundaries and exact matching, category precedence, distinct same-name Go methods and nested declarations, group scores under repeated evidence, and exact Markdown duplicates alongside identical-looking source and near-duplicate prose.

The original branch had two formatter tests that depended on an ignored, missing HTML sample. Test-only commit `88ad003` makes both tests self-contained with a tracked HTML fixture. The commit contains only formatter tests and the sample fixture.

## Separate parser investigation

The original and experimental standard profiles report these seven JavaScript and TypeScript bracket warnings on the full corpus.

- `website/.vitepress/blog.data.ts`
- `website/.vitepress/components/homeExample.data.ts`
- `website/.vitepress/config.ts`
- `website/.vitepress/githubStars.data.ts`
- `website/.vitepress/plugins/local-search.ts`
- `website/docsearch.config.js`
- `website/scripts/check-markdown-links.mjs`

All standard and documentation-retaining profiles emitted the same warning list. Each warning says the outline may be incomplete. The experiment establishes the affected files and matching diagnostics. Diagnosing the lexical cause is a separate investigation. [Captured warnings](experiment/data/warnings.json)

## Reproduce

Run from the experimental worktree on Windows with PowerShell, Go and Python. The research path points to the workspace containing `repro/`, `checkouts/`, `bin/` and `outputs/`. The example is relative to the experimental worktree in this session.

```powershell
./experiment/build.ps1
$research = (Resolve-Path ../../../retrieval-research).Path
python experiment/measure.py --research $research
python experiment/measure.py --research $research --profile-set docs-retained

$env:GOCACHE = (Resolve-Path testdb/experiment-artifacts/go-cache).Path
$env:CODECORE_EXPERIMENT_CORPUS = Join-Path $research 'checkouts/task-full-canonical'
$env:CODECORE_EXPERIMENT_INDEX_OUTPUT = Join-Path (Get-Location).Path 'testdb/experiment-artifacts/index-stats.json'
go test -run '^TestMeasureCorpusIndex$' -count=1 -v .
```

The build script tests and vets each pinned stage in a detached worktree and produces four named executables under `testdb/experiment-artifacts/bin`. The runner saves raw CLI output and measurements under `testdb/experiment-artifacts/run` and `run-docs`. The committed measurements are linked below.

- [Standard metrics and anchor ranks](experiment/data/summary.tsv), [standard timing samples](experiment/data/samples.tsv) and [standard provenance and flags](experiment/data/metadata.json).
- [Metrics and anchor ranks with documentation retained](experiment/data/docs-summary.tsv), [timing samples](experiment/data/docs-samples.tsv) and [provenance and flags](experiment/data/docs-metadata.json).
- [Canonical CLI replay](experiment/data/regressions.json) and [option regression checks](experiment/data/option-checks.json).
