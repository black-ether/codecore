# Contextual ranking experiment

The four opt-in prototypes produced modest rank changes on Task. The checksum discovery query still missed through rank 10, and natural-query hits stayed flat on held-out Task and PowerPath. The branch retains the opt-in score diagnostics. The scoring prototypes are available from their measured commit.

## Ranking diagnostics

Codecore scores every retained outline row together with any attached comments. Each row is a BM25 document. With identifier splitting enabled, original lexemes determine document length and indexed aliases provide additional matches. The average length is computed over the searched corpus, including rows that later fail the definition filter. BM25 uses `k1 = 1.2` and `b = 0.75`. Holding term frequency constant, shorter rows get a larger contribution. Holding document length constant, additional occurrences increase the score with diminishing returns.

For each query term, Codecore unions the posting lists for the exact and eligible prefix matches. It uses the maximum frequency of any matching indexed token in each row. The union’s document frequency sets the inverse document frequency `log(1 + (N - df + 0.5) / (df + 0.5))`. Terms at least three bytes long can use prefixes unless `--word` is active. They get a separate three-point bonus for an original whole-word match. The name extracted for a definition contributes two additional occurrences of each name term without adding to document length. With identifier splitting, only whole name terms receive this boost.

These choices favor short lines or assignments that repeat a rare exact term. A function’s name and comments compete as part of its header row. Vocabulary elsewhere in the body scores independently. Multiple occurrences of the same query word currently contribute again to the legacy score. The diagnostics report both the occurrence-by-occurrence contributions and distinct term coverage.

`--def-only` uses a lexical definition heuristic. A definition keyword such as `var`, `const`, `let` or `func` qualifies the row. A single `=` delimiter also qualifies an assignment when it passes the comparison guard. The option includes variable and property assignments by design. The filter runs after scoring, so the underlying corpus statistics stay the same.

Grouped retrieval’s `unit.end` is still the final retained outline row. The explicit structural outline’s `endLine` uses the source closing boundary and, for Go declarations, accounts for the complete declaration. In the pinned Task source, `(*ChecksumChecker).IsUpToDate` groups as lines 31–73 while the explicit source outline reports 31–74, including the closing brace. Both point to the same heading. A multiline signature can make the two ends diverge by more than one line. The comparison keeps legacy ranked spans and uses the representative’s actual `line` for direct hits.

The diagnostic build adds `--explain-ranking` for ranked JSON. For each returned statement it exposes the document and mean lengths, document count, definition-name terms, distinct coverage, and each term’s effective TF, DF, IDF, BM25 and whole-word contribution. Prototype adjustments and supporting terms appear separately when enabled. Diagnostics are off by default and leave ranked scores and ordering unchanged.

## Independent prototypes

Each mode starts with the production-filtered, identifier-split, grouped configuration and changes one scoring rule. The prototype build accepts `--rank-mode coverage|context|unit|category`. Distinct query terms are used for all four modes, so repeated query words cannot amplify their scores. The production and grouping controls use the exact historical executables.

| Mode | Scoring rule |
| --- | --- |
| Coverage | Multiplies the line score by a factor from 1 to 2 as distinct local coverage goes from one term to every query term. |
| Context | Rewards missing terms found in the nearest named function or declaration’s name and attached comments. The line must already match. The maximum bonus is six points, in proportion to newly covered terms. |
| Unit | Keeps the strongest lexical line and up to three sibling evidence rows that each add a previously uncovered term. This compact representation uses the same capped six-point bonus. It retains source lines and excerpts for the supporting evidence. |
| Category | Reduces a one-term import match by 35% when an eligible non-import result covers at least two terms. Single-term queries and imports matching multiple terms keep their scores. Tests, configuration and other categories keep their lexical scores. |

The unit and context mechanisms use existing outline scopes. Supporting a term repeatedly adds no additional score and the boost does not grow with body size. Large functions can still offer more opportunities to find useful or coincidental evidence. Category weighting tests recognizable imports and leaves generic boilerplate to the lexical ranker.

## Evaluation method

The runner loads the original fixed and held-out Task queries and anchors from the existing reproduction infrastructure. The four unmodified exact-identifier controls retain their original query wording. The diagnostic checksum query adds a separate case. PowerPath is checked at `210d6a1b63144ff0533ab89737f043c6dc7c3ce7`, with fixed queries and line anchors for stored conditions and ambiguous selection. Task is verified at the original 982-file canonical pin. [PowerPath queries and anchors](experiment/contextual_powerpath.json)

The matrix runs top 5 and top 10 separately with one warm-up and three captured timed samples, shuffling the profile order using a fixed seed. Timing includes process startup, parsing, ranking and JSON output. Coverage diagnostics are collected in untimed runs and matched against the regular output. A direct hit requires the primary evidence line to fall inside an anchor. An enclosing span can overlap an anchor while the primary line sits outside it. Additional evidence from unit ranking gets a separate support-hit column. Relevant implementation units are distinct named Go or PowerShell functions with an anchored result. Repeat counts measure extra results in the same named implementation regardless of relevance.

## Results at top 5 and top 10

### The checksum failure

For **checksum freshness skip execution**, the unfiltered original ranks two copies of a conditional-execution documentation heading first. The heading matches two terms, with BM25 contributions of 5.08 for `skip` and 5.12 for `execution`, plus 6.00 in whole-word bonuses. It scores 16.21. Installer and remote include checksums lead the production-filtered results.

| Production result | Rank | Doc length | Covered terms | Effective TF and DF | BM25 + bonus |
| --- | ---: | ---: | --- | --- | ---: |
| Remote include checksum assignment, `taskfile/ast/include.go:208` | 1 | 4 | `checksum` | 4, 130 | 9.00 + 3.00 |
| Installer checksum assignment, `install-task.sh:377` | 3 | 3 | `checksum` | 3, 130 | 8.82 + 3.00 |
| Local digest computation, `sources_checksum.go:95` | 20 | 9 | `checksum` | 4, 130 | 7.64 + 3.00 |
| Execution wrapper call, `task.go:206` | 184 | 11 | `execution` | 1, 54 | 4.04 + 0.00 |
| Freshness decision, `sources_checksum.go:31` | 185 | 9 | `checksum` | 1, 130 | 3.88 + 0.00 |

Of the four query terms, the freshness header matches only `checksum` through its receiver type. It receives zero whole-word bonus. The assignment has two literal occurrences and a definition-name boost, which takes TF to four. In the informed S4 query, the same freshness header ranks first with 26.01 points. It covers three terms, and the boosted exact name `IsUpToDate` contributes 10.28 BM25 points plus 3.00 for the exact term. The N3 trace shows how repeated query words affect the score. The `task` occurrence contributes 4.73 twice to the digest function’s 20.10 score. [Focused score traces](experiment/data/contextual/diagnostics.json)

Every variant missed the selected checksum anchors through rank 10. Coverage moved `(*Executor).startExecution` to rank 1 by matching `skip` and `execution` on its log line at `task.go:495`. That code checks for an already-running task. The checksum freshness decision is at a separate source anchor.

### Fixed Task natural queries

Paired columns show top 5 / top 10 across the four fixed natural queries. Relevant units and repeats are summed per query. Median runtime is the median of the four top-10 query medians. These queries produced zero enclosing-only span hits in every top-10 list.

| Profile | Selected direct hits | Distinct relevant units | Repeated implementation hits | Top-10 stdout bytes | Top-10 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original | 2 / 2 | 1 / 1 | 2 / 3 | 13,309 | 477.3 |
| Identifiers + production | 5 / 7 | 2 / 2 | 3 / 6 | 16,970 | 250.1 |
| Identifiers + production + grouping | 2 / 2 | 2 / 2 | 0 / 0 | 16,261 | 258.0 |
| Coverage | 3 / 3 | 3 / 3 | 0 / 0 | 16,368 | 253.7 |
| Enclosing context | 2 / 3 | 2 / 3 | 0 / 0 | 16,106 | 268.3 |
| Compact units | 3 / 3 | 3 / 3 | 0 / 0 | 18,188 | 264.0 |
| Import weighting | 2 / 3 | 2 / 3 | 0 / 0 | 16,154 | 256.4 |

**Coverage** raised mean local query-term coverage from 2.10 to 2.45 in the fixed natural top-five lists. Its extra implementation was another merge function for N2. On the held-out informed output-buffer query, the direct hit moved from rank 7 to 5. The natural held-out pair stayed at one selected hit in each limit.

**Enclosing context** produced no additional top-five fixed natural hit. It added no natural hits on held-out Task or PowerPath. On fixed Task informed queries, average effective term coverage through 10 moved from 1.88 to 2.00, with the same eight selected results and eight distinct relevant implementations as grouping.

**Compact units** demonstrated a benefit for evidence presentation. S4 returned the rank-1 freshness function with the missing-output, digest-write and dry-guard lines at 61, 48 and 46 as supporting evidence. Across the four fixed informed top-five lists, selected results rose from six to seven and enclosing span hits rose from one to two. The fixed natural top-10 output grew by 1,927 bytes relative to grouping. Unit ranking also pushed two import results into D2’s top 10, which previously contained zero.

**Import weighting** moved one import out of the top 10 for the held-out signal identifier query HD1, from four imports to three. A `pnpm-lock.yaml` line entered the list and discovery still centered on the same rank-1 signal registration. This result gives no evidence for general boilerplate weighting.

### Distinct-query ablation

A separate control sent the same ASCII query tokens in their original order and omitted repeated tokens. Grouped scoring put the second N2 merge implementation at rank 7 under that control, matching the enclosing-context and category results. Coverage reached rank 4 and compact units reached rank 2. The context and category top-10 gains on the fixed natural suite are explained by query de-duplication alone. The checksum and both held-out natural suites kept their selected-hit counts in this control. [Distinct-query ablation](experiment/data/contextual/ablation-summary.tsv)

### Transfer and exact-identifier controls

No prototype improved selected natural hits on held-out Task or PowerPath at either limit. The PowerPath ambiguity prompt’s matching statement stayed at rank 3 in every profile. The natural stored-condition query missed through rank 10. Its informed counterpart found the target function with every grouped ranker, and compact units supplied a supporting line.

The four fixed and held-out Task exact-identifier controls and two PowerPath identifier controls kept the same rank-1 source locations in every profile. The Task controls were direct hits. The two PowerPath controls returned the function headings with enclosing spans. Grouping generally removed secondary direct matches in the same function. Compact units preserved some of that evidence as separately labeled support.

## Disposition

Retain the score diagnostics from `c3ccfaf` and archive the scoring prototypes in `d515070`. The small gains on merge and informed queries did not transfer to natural queries on the held-out subsystems or PowerPath. The evidence did not justify a combined scorer or a new scoring option on the branch tip. The four existing retrieval options and the explicit outline ranges remain separately available. Both experiment builds reproduced all 94 original canonical commands byte-for-byte. The diagnostics build matched the historical Stage 4 output in twelve existing-flag cases and matched both text and JSON output from the explicit-range executable on the checksum source file. [CLI regression checks](experiment/data/contextual/regressions.json)

## Reproduce

From the experimental worktree, supply the existing research workspace to the runner. PowerShell and Go build the pinned diagnostics and prototype revisions into separate executables. If the prior stage executables are absent, the same build script without `-Contextual` rebuilds those pins.

```powershell
./experiment/build.ps1 -Contextual
$research = (Resolve-Path ../../../retrieval-research).Path
python experiment/contextual_measure.py --research $research
python experiment/contextual_measure.py --research $research --ablation
```

The runner saves stdout, stderr and score traces under `testdb/experiment-artifacts/contextual-run/raw`. The committed data records per-query metrics and ranks, timing samples, binary identities and focused score contributions. The ablation writes to `contextual-ablation` and has [its own timing samples](experiment/data/contextual/ablation-samples.tsv) and [provenance](experiment/data/contextual/ablation-metadata.json). [Metrics](experiment/data/contextual/summary.tsv) · [Timing samples](experiment/data/contextual/samples.tsv) · [Provenance](experiment/data/contextual/metadata.json) · [Score contributions](experiment/data/contextual/diagnostics.json)
