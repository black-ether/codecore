"""Compare independent contextual rankers against the pinned retrieval benchmark."""

import argparse
from collections import Counter, defaultdict
import json
from pathlib import Path, PureWindowsPath
import random
import re
import statistics
import sys
import time

sys.dont_write_bytecode = True
import measure as prior

POWERPATH = Path("checkouts/powerpath")
POWERPATH_PIN = "210d6a1b63144ff0533ab89737f043c6dc7c3ce7"
POWERPATH_SOURCE = "Lib/PowerPath/PP.Execution.psm1"
POWERPATH_SHA256 = "e01386c2e15c998b930de749a9856bdf83f9ccd8d74cc187e0623a35c9a54659"
PRODUCTION = prior.IDENTIFIERS + prior.PRODUCTION
GROUPED = PRODUCTION + prior.STRUCTURE
MODES = ("coverage", "context", "unit", "category")
PROFILES = {
    "original": (0, ()),
    "production": (1, PRODUCTION),
    "grouped": (2, GROUPED),
    **{mode: (4, GROUPED + ("--rank-mode", mode)) for mode in MODES},
}
ABLATION = {"grouped": (2, GROUPED), "distinct-query": (2, GROUPED)}
EXAMPLE = "checksum freshness skip execution"


def verify_powerpath(root):
    assert prior.run(["git", "rev-parse", "HEAD"], root).stdout.decode().strip() == POWERPATH_PIN
    assert not prior.run(["git", "status", "--porcelain"], root).stdout
    assert prior.sha256(root / POWERPATH_SOURCE) == POWERPATH_SHA256


def cases(research):
    task = prior.load_cases(research)
    for case in task.values():
        case["corpus"] = prior.CORPUS
    task["X1"] = dict(kind="natural", suite="diagnostic", query=EXAMPLE, corpus=prior.CORPUS,
                      anchors={key: value for key, value in task["N3"]["anchors"].items()
                               if key in {"B-gate", "B-digest", "B-cleanup"}})
    cross = prior.load(prior.ROOT / "experiment/contextual_powerpath.json")
    for key, item in cross["queries"].items():
        task[key] = dict(kind=item["kind"], suite="cross-repository", query=item["query"], corpus=POWERPATH,
                         anchors={anchor: cross["anchors"][anchor] for anchor in item["anchors"]})
    return task


def relative(filename, corpus):
    return PureWindowsPath(filename).relative_to(PureWindowsPath(corpus)).as_posix()


def is_implementation(unit):
    return unit is not None and unit[2] == "function"


def powerpath_index(exe, research):
    result = {}
    response = prior.run([exe, "--format", "json", "--depth", "10000", str(POWERPATH)], research)

    def walk(file, nodes, owner=None):
        for node in nodes:
            line = node["line"]
            words = node["text"].split(maxsplit=1)
            unit = (file, line, "function") if words and words[0].lower() in {"function", "filter"} else owner
            if unit is not None:
                result[file, line] = unit
            walk(file, node.get("children", ()), unit)

    for outline in json.loads(response.stdout):
        if outline["language"] == "powershell":
            walk(relative(outline["file"], POWERPATH), outline["nodes"])
    return result


def assess(rows, traces, case, units):
    ranks = defaultdict(lambda: defaultdict(list))
    implementation_hits = set()
    implementation_counts = Counter()
    direct_coverage, effective_coverage = [], []
    import_results = 0
    locations = []
    for row, trace in zip(rows, traces, strict=True):
        rank, file, line = row["rank"], relative(row["file"], case["corpus"]), row["line"]
        unit = units.get((file, line))
        if is_implementation(unit):
            implementation_counts[unit] += 1
        import_results += (unit is not None and unit[2] == "import") or row["text"].startswith("import ")
        locations.append(f"{file}:{line}")
        start, end = row.get("unit", {}).get("start", line), row.get("unit", {}).get("end", row["end"])
        for anchor, ranges in case["anchors"].items():
            direct = any(file == path and low <= line <= high for path, low, high in ranges)
            span = any(file == path and start <= high and end >= low for path, low, high in ranges)
            support = any(file == path and low <= hit["line"] <= high
                          for path, low, high in ranges for hit in row.get("support", ()))
            if direct or span:
                ranks[anchor]["direct" if direct else "span"].append(rank)
                if is_implementation(unit):
                    implementation_hits.add(unit)
            if support:
                ranks[anchor]["support"].append(rank)
        info = trace["explanation"]
        direct_coverage.append(info["covered"])
        effective_coverage.append(info["covered"] + len(info.get("supportingTerms", ())))
    by_kind = {kind: {rank for hit in ranks.values() for rank in hit.get(kind, ())}
               for kind in ("direct", "span", "support")}
    selected = by_kind["direct"] | by_kind["span"]
    out = dict(
        results=len(rows), selected=len(selected), direct=len(by_kind["direct"]),
        span=len(by_kind["span"]), support_hits=len(by_kind["support"]),
        distinct_relevant_units=len(implementation_hits),
        distinct_implementation_units=len(implementation_counts),
        repeated_implementation_hits=sum(count - 1 for count in implementation_counts.values()),
        import_results=import_results,
        mean_matching_terms=f"{statistics.mean(direct_coverage) if rows else 0:.3f}",
        mean_terms_with_support=f"{statistics.mean(effective_coverage) if rows else 0:.3f}",
        anchor_ranks=json.dumps(ranks, separators=(",", ":")),
        first_locations=" | ".join(locations),
    )
    return out


def command(exe, case, flags, limit, explain=False, distinct=False):
    diagnostics = ("--explain-ranking",) if explain else ()
    query = case["query"]
    if distinct:
        # The fixed query set is ASCII. This mirrors whole-identifier
        # boundaries before Codecore applies its usual stopword filter.
        assert query.isascii()
        query = " ".join(dict.fromkeys(re.findall(r"[a-z0-9_]+", query.lower())))
    return [exe, "--ask", query, "--top", str(limit), "--format", "json", *flags, *diagnostics, str(case["corpus"])]


def check_trace(ranked, evidence):
    assert len(ranked) == len(evidence)
    for row, traced in zip(ranked, evidence, strict=True):
        assert row == {key: val for key, val in traced.items() if key != "explanation"}, (row, traced)


def collect_diagnostics(output, research, exe, queries):
    records = []
    picks = {"internal/fingerprint/sources_checksum.go": {31, 95}, "task.go": {206, 225, 226}}
    for key in ("X1", "N3", "S4"):
        for profile, flags in (("legacy", ()), ("split-production", PRODUCTION), ("grouped", GROUPED)):
            case = queries[key]
            result = prior.run(command(exe, case, flags, 1000, explain=True), research)
            raw = json.loads(result.stdout) or []
            focused = [row for row in raw if row["rank"] <= 5 or
                       row["line"] in picks.get(relative(row["file"], case["corpus"]), ())]
            records.append(dict(query=key, text=case["query"], profile=profile, results=focused))
    prior.write_json(output / "diagnostics.json", records)


def verify_compatibility(output, research, diagnostics, prototype, queries):
    reference = research / "outputs/full-corpus-canonical"
    replays = {}
    for name, exe in (("diagnostics", diagnostics), ("prototypes-default", prototype)):
        result = prior.replay(research, [exe], reference)[0]
        replays[name] = result["commands"]
    legacy_flags = {
        "all": GROUPED + prior.DUPLICATES,
        "dedup-only": prior.DUPLICATES,
        "grouping-only": prior.STRUCTURE,
    }
    cases_checked = 0
    for key in ("N1", "N3", "D1", "D2"):
        for flags in legacy_flags.values():
            before = prior.run(command(diagnostics, queries[key], flags, 10), research)
            after = prior.run(command(prototype, queries[key], flags, 10), research)
            assert (before.stdout, before.stderr) == (after.stdout, after.stderr), (key, flags)
            cases_checked += 1
    prior.write_json(output / "regressions.json", dict(
        canonical_byte_identical=replays, legacy_experimental_flag_cases=cases_checked))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--research", type=Path, required=True)
    parser.add_argument("--artifacts", type=Path, default=prior.ROOT / "testdb/experiment-artifacts")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--samples", type=int, default=3)
    parser.add_argument("--ablation", action="store_true", help="compare grouped ranking with distinct query tokens alone")
    args = parser.parse_args()
    research, artifacts = args.research.resolve(), args.artifacts.resolve()
    profiles = ABLATION if args.ablation else PROFILES
    output = (args.output or artifacts / ("contextual-ablation" if args.ablation else "contextual-run")).resolve()
    if args.samples < 1 or output.is_relative_to(research):
        parser.error("use a separate output directory and at least one sample")
    raw = output / "raw"
    raw.mkdir(parents=True, exist_ok=True)
    tracked = prior.verify_corpus(research / prior.CORPUS)
    verify_powerpath(research / POWERPATH)
    binaries = [
        research / prior.ORIGINAL,
        *(artifacts / "bin" / name for name in (prior.BUILDS[1], prior.BUILDS[2],
           "codecore-ranking-diagnostics.exe", "codecore-contextual-prototypes.exe")),
    ]
    previous = prior.load(prior.ROOT / "experiment/data/metadata.json")["binaries"]
    assert prior.sha256(binaries[0]) == prior.ORIGINAL_SHA256
    prior_stages = prior.load(prior.ROOT / "experiment/stages.json")
    stage_pins = prior.load(prior.ROOT / "experiment/contextual_stages.json")
    metadata = []
    for i, exe in enumerate(binaries):
        version = prior.run(["go", "version", "-m", exe], prior.ROOT).stdout.decode().splitlines()[1:]
        if i > 0:
            pin = prior_stages[i] if i < 3 else stage_pins[i - 3]
            assert "\tbuild\tvcs.modified=false" in version
            assert f"\tbuild\tvcs.revision={pin['commit']}" in version
        entry = dict(binary=exe.name, sha256=prior.sha256(exe), go_version=version)
        if i < 3:
            historical = previous[0 if i == 0 else i + 1]
            entry["matches_prior_sha256"] = entry["sha256"] == historical["sha256"]
        metadata.append(entry)
    prior.write_json(output / "metadata.json", dict(
        corpus_commit=prior.PIN, verified_task_blobs=tracked, powerpath_commit=POWERPATH_PIN,
        powerpath_anchor_file=POWERPATH_SOURCE, powerpath_anchor_sha256=POWERPATH_SHA256,
        samples=args.samples, seed=4837, limits=(5, 10), profiles=profiles, binaries=metadata))
    queries = cases(research)
    units = {
        prior.CORPUS: prior.go_unit_index(binaries[0], research),
        POWERPATH: powerpath_index(binaries[0], research),
    }
    reference = research / "outputs/full-corpus-canonical"
    rows, timing = [], []
    order = random.Random(4837)
    for key, case in queries.items():
        query_units = units[case["corpus"]]
        warm = {}
        elapsed = defaultdict(list)
        order_choices = [(mode, limit) for mode in profiles for limit in (5, 10)]
        for sample in range(args.samples + 1):
            order.shuffle(order_choices)
            for position, (mode, limit) in enumerate(order_choices, 1):
                build, flags = profiles[mode]
                argv = command(binaries[build], case, flags, limit, distinct=mode == "distinct-query")
                start = time.perf_counter_ns()
                result = prior.run(argv, research)
                ms = (time.perf_counter_ns() - start) / 1e6
                pair = result.stdout, result.stderr
                if sample == 0:
                    warm[mode, limit] = pair
                    (raw / f"{key}-{mode}-{limit}.stdout").write_bytes(result.stdout)
                    (raw / f"{key}-{mode}-{limit}.stderr").write_bytes(result.stderr)
                    if mode == "original" and case["suite"] == "fixed":
                        assert pair == tuple((reference / f"{key}-rank-{limit}.{stream}").read_bytes()
                                             for stream in ("stdout", "stderr")), key
                else:
                    assert pair == warm[mode, limit], (key, mode, limit, sample)
                    elapsed[mode, limit].append(ms)
                    timing.append(dict(query=key, profile=mode, top=limit, sample=sample,
                                       order=position, duration_ms=f"{ms:.3f}"))
        for mode, (build, flags) in profiles.items():
            instrumented = binaries[3] if build < 3 else binaries[build]
            trace_response = prior.run(command(instrumented, case, flags, 10, explain=True, distinct=mode == "distinct-query"), research)
            trace = json.loads(trace_response.stdout) or []
            (raw / f"{key}-{mode}-trace.json").write_bytes(trace_response.stdout)
            for limit in (5, 10):
                stdout, stderr = warm[mode, limit]
                ranked = json.loads(stdout) or []
                limited_trace = trace[:limit]
                check_trace(ranked, limited_trace)
                rows.append(dict(query=key, suite=case["suite"], kind=case["kind"], profile=mode, top=limit,
                                 stdout_bytes=len(stdout), stderr_bytes=len(stderr),
                                 median_ms=f"{statistics.median(elapsed[mode, limit]):.3f}",
                                 **assess(ranked, limited_trace, case, query_units)))
        if key == next(iter(queries)) or len(rows) % (3 * len(profiles) * 2) == 0:
            prior.write_tsv(output / "summary.tsv", rows)
            prior.write_tsv(output / "samples.tsv", timing)
        print(f"{key}: top 5 and 10 compared across {len(profiles)} profiles", flush=True)

    prior.write_tsv(output / "summary.tsv", rows)
    prior.write_tsv(output / "samples.tsv", timing)
    if not args.ablation:
        collect_diagnostics(output, research, binaries[3], queries)
        verify_compatibility(output, research, binaries[3], binaries[4], queries)
    assert not prior.run(["git", "status", "--porcelain"], research / prior.CORPUS).stdout
    verify_powerpath(research / POWERPATH)


if __name__ == "__main__":
    main()
