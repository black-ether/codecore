"""Run the Codecore retrieval experiment against the unchanged research workspace."""

import argparse
from collections import defaultdict
import csv
import hashlib
import json
import os
from pathlib import Path, PureWindowsPath
import random
import re
import statistics
import subprocess
import time

ROOT = Path(__file__).resolve().parents[1]
CORPUS = Path("checkouts/task-full-canonical")
PIN = "b5aef1c287a9be8038347a17646bdf4bc7185bce"
ORIGINAL = "bin/codecore-26b8bde.exe"
ORIGINAL_SHA256 = "2581653966ac017d4f4ad1008902247c456f220178022ad045fc80f62c355178"
BUILDS = (
    "codecore-stage1-identifiers.exe",
    "codecore-stage2-categories.exe",
    "codecore-stage3-structure.exe",
    "codecore-stage4-duplicates.exe",
)
IDENTIFIERS = ("--split-identifiers",)
PRODUCTION = ("--categories", "production")
STRUCTURE = ("--group-structure",)
DUPLICATES = ("--dedup-docs",)
PROFILES = {
    "original": (0, ()),
    "identifiers": (1, IDENTIFIERS),
    "categories-only": (2, PRODUCTION),
    "identifiers-categories": (2, IDENTIFIERS + PRODUCTION),
    "grouping-only": (3, STRUCTURE),
    "identifiers-categories-grouping": (3, IDENTIFIERS + PRODUCTION + STRUCTURE),
    "dedup-only": (4, DUPLICATES),
    "all": (4, IDENTIFIERS + PRODUCTION + STRUCTURE + DUPLICATES),
}
DOCS_RETAINED = IDENTIFIERS + ("--categories", "production,docs") + STRUCTURE
DOCS_PROFILES = {
    "docs-before-dedup": (3, DOCS_RETAINED),
    "docs-after-dedup": (4, DOCS_RETAINED + DUPLICATES),
}
INVENTORY = re.compile(r"^func\s+\S.*[\[(]|^type\s+\S.*(?:struct|interface)\s*\{|^(?:import|var|const|type)\s+\(")
GO_FUNCTION = re.compile(r"^func\s+(?:\([^)]*\)\s*)?[\w]+\s*[\[(]")


def run(argv, cwd):
    return subprocess.run(argv, cwd=cwd, capture_output=True, check=True, timeout=60)


def load(path):
    return json.loads(path.read_bytes())


def write_json(path, data):
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def write_tsv(path, data):
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, list(data[0]), delimiter="\t", lineterminator="\n")
        writer.writeheader()
        writer.writerows(data)


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify_corpus(path):
    assert run(["git", "rev-parse", "HEAD"], path).stdout.decode().strip() == PIN
    assert not run(["git", "status", "--porcelain"], path).stdout
    tracked = run(["git", "ls-files", "--stage", "-z"], path).stdout.split(b"\0")[:-1]
    assert len(tracked) == 982
    for entry in tracked:
        info, name = entry.split(b"\t", 1)
        mode, digest, stage = info.decode().split()
        assert mode in {"100644", "100755", "120000"} and stage == "0", entry
        working = path / name.decode()
        blob = Path(os.readlink(working)).as_posix().encode() if mode == "120000" and working.is_symlink() else working.read_bytes()
        assert hashlib.sha1(f"blob {len(blob)}\0".encode() + blob).hexdigest() == digest, name
    return len(tracked)


def relative_file(filename):
    return PureWindowsPath(filename).relative_to(PureWindowsPath(CORPUS)).as_posix()


def load_cases(research):
    anchors = load(research / "repro/anchors.json")
    cases = {}
    for source in ["natural-queries.json", "informed-queries.json"]:
        for key, query in load(research / "repro" / source).items():
            kind = {"N": "natural", "S": "informed", "D": "exact-identifier"}[key[0]]
            prefix = "A" if key in {"N1", "N2", "S1", "S2", "D1"} else "B"
            details = {"A": ("colon", "env"),
                       "B": ("sort", "err-hash", "err-write", "err-output-missing",
                             "err-output-other", "err-read-old")}[prefix]
            cases[key] = dict(kind=kind, suite="fixed", query=query["ask"],
                              anchors={k: v for k, v in anchors.items() if k.startswith(prefix + "-") or k in details})
    held = load(ROOT / "experiment/held-out.json")
    for key, case in held["queries"].items():
        cases[key] = dict(kind=case["kind"], suite="held-out", query=case["query"],
                          anchors={k: held["anchors"][k] for k in case["anchors"]})
    return cases


def go_unit_index(original, cwd):
    """Common comparison metric from the original outline, independent of --group-structure."""
    response = run([original, "--format", "json", "--depth", "10000", str(CORPUS)], cwd)
    result = {}

    def walk(file, nodes, unit=None):
        for node in nodes:
            line, text = node["line"], node["text"]
            owner = unit
            if INVENTORY.match(text):
                kind = "import" if text.startswith("import") else "function" if GO_FUNCTION.match(text) else "declaration"
                owner = (file, line, kind)
            if owner is not None:
                result[file, line] = owner
            walk(file, node.get("children", ()), owner)

    for outline in json.loads(response.stdout):
        if outline["language"] == "go":
            walk(relative_file(outline["file"]), outline["nodes"])
    return result


def score(rows, anchors, units):
    ranks = defaultdict(lambda: defaultdict(list))
    identities = set()
    named = set()
    imports = 0
    locations = []
    duplicates = defaultdict(set)
    for rank, row in enumerate(rows, 1):
        assert row["rank"] == rank, row
        file, line = relative_file(row["file"]), row["line"]
        key = units.get((file, line), (file, line, "line"))
        identities.add(key)
        if key[2] == "function":
            named.add(key)
        imports += key[2] == "import" or row["text"].startswith("import ")
        locations.append(f"{file}:{line}")
        if file.lower().endswith((".md", ".markdown", ".mdx")) and len(row["text"]) >= 32:
            duplicates[row["text"]].add(file)
        unit = row.get("unit", {})
        start, end = unit.get("start", line), unit.get("end", row["end"])
        for anchor, ranges in anchors.items():
            if any(file == path and low <= line <= high for path, low, high in ranges):
                ranks[anchor]["direct"].append(rank)
            elif any(file == path and start <= high and end >= low for path, low, high in ranges):
                ranks[anchor]["span"].append(rank)
    direct = {rank for modes in ranks.values() for rank in modes.get("direct", ())}
    span = {rank for modes in ranks.values() for rank in modes.get("span", ())}
    return dict(results=len(rows), selected_hits=len(direct | span), direct_hit_results=len(direct),
                span_hit_results=len(span), distinct_units=len(identities), distinct_go_functions=len(named),
                import_results=imports, repeated_markdown_lines=sum(len(files) - 1 for files in duplicates.values()),
                anchor_ranks=json.dumps(ranks, separators=(",", ":")), first_five=" | ".join(locations[:5]))


def replay(research, executables, reference):
    commands = [cmd for cmd in load(reference / "commands.json") if cmd["argv"][0] == ORIGINAL]
    verified = []
    for stage, executable in enumerate(executables, 1):
        # Earlier stages replay ten rank-50 commands each. The last stage
        # replays the entire original Codecore command manifest.
        cases = commands if stage == len(executables) else [cmd for cmd in commands if cmd["id"].endswith("-rank-50")]
        for cmd in cases:
            argv = [executable, *cmd["argv"][1:]]
            got = run(argv, research / cmd["cwd"])
            for kind in ("stdout", "stderr"):
                assert getattr(got, kind) == (reference / f'{cmd["id"]}.{kind}').read_bytes(), (stage, cmd["id"], kind)
            assert got.returncode == cmd["exit"], (stage, cmd["id"])
        verified.append(dict(stage=stage, commands=len(cases), status="byte-identical stdout and stderr"))
    return verified


def check_options(research, executable, reference, output):
    all_categories = ("--categories", "production,docs,tests,fixtures,generated")
    command = {cmd["id"]: cmd for cmd in load(reference / "commands.json")}
    verified = []
    for key in ("N1-rank-50", "N1-notests-20"):
        argv = command[key]["argv"]
        got = run([executable, *argv[1:-1], *all_categories, argv[-1]], research)
        assert got.stdout == (reference / f"{key}.stdout").read_bytes(), key
        assert got.stderr == (reference / f"{key}.stderr").read_bytes(), key
        verified.append(key + " with all categories")

    root = output / "_cli-probe"
    root.mkdir(exist_ok=True)
    sources = {
        "implementation.go": ("production", "const witness = 1"),
        "second.go": ("production", "const witness = 1"),
        "guide.md": ("docs", "A documentation witness with identical text across guide versions."),
        "other.md": ("docs", "A documentation witness with identical text across guide versions."),
        "implementation_test.go": ("tests", "const witness = 1"),
        "fixtures/data.yml": ("fixtures", "witness: 1"),
        "mocks/client.go": ("generated", "const witness = 1"),
        "header.go": ("generated", "// Code generated by Example. DO NOT EDIT.\nconst witness = 1"),
    }
    for name, (_, source) in sources.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(source + "\n", encoding="utf-8")

    def files(flags):
        got = run([executable, "--ask", "witness", "--top", "30", "--format", "json", *flags, str(root)], research)
        return {Path(row["file"]).relative_to(root).as_posix() for row in json.loads(got.stdout)}

    for category in sorted({category for category, _ in sources.values()}):
        expected = {name for name, (label, _) in sources.items() if label == category}
        assert files(("--categories", category)) == expected, category
        verified.append("synthetic " + category + " filter")
    assert files(DUPLICATES) == set(sources) - {"other.md"}
    verified.append("dedup preserves distinct source files")
    grouped = root / "structure.go"
    grouped.write_text(
        "package probe\nfunc FindHTTP2Messages() {\n repeatedNeedle()\n repeatedNeedle()\n}\n",
        encoding="utf-8",
    )
    match = [executable, "--ask", "messages", "--word", *IDENTIFIERS, "--format", "json", str(grouped)]
    assert json.loads(run(match, research).stdout)[0]["line"] == 2
    verified.append("identifier component through --word")
    match = [executable, "--ask", "repeatedNeedle", *STRUCTURE, str(grouped)]
    text = run(match, research).stdout.decode()
    assert text.count("match L:") == 1 and "[L:2-4] function FindHTTP2Messages" in text
    verified.append("grouped text range and representative line")
    for option in ("--group-structure", "--dedup-docs"):
        got = subprocess.run([executable, "--ask", "witness", "--count", option, str(CORPUS)],
                             cwd=research, capture_output=True, timeout=60)
        assert got.returncode == 2 and option.encode() in got.stderr
        verified.append(option + " rejects --count")
    return verified


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--research", required=True, type=Path)
    parser.add_argument("--artifacts", type=Path, default=ROOT / "testdb/experiment-artifacts")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--samples", type=int, default=5)
    parser.add_argument("--profile-set", choices=("standard", "docs-retained"), default="standard")
    args = parser.parse_args()
    research = args.research.resolve()
    artifacts = args.artifacts.resolve()
    retained_docs = args.profile_set == "docs-retained"
    profiles = DOCS_PROFILES if retained_docs else PROFILES
    output = (args.output or artifacts / ("run-docs" if retained_docs else "run")).resolve()
    if output.is_relative_to(research) or args.samples < 1:
        parser.error("use a separate output directory and at least one timed sample")
    output.mkdir(parents=True, exist_ok=True)
    raw = output / "raw"
    raw.mkdir(exist_ok=True)
    reference = research / "outputs/full-corpus-canonical"
    tracked = verify_corpus(research / CORPUS)
    binaries = [research / ORIGINAL, *(artifacts / "bin" / name for name in BUILDS)]
    assert sha256(binaries[0]) == ORIGINAL_SHA256
    metadata = dict(corpus=CORPUS.as_posix(), corpus_commit=PIN, verified_tracked_blobs=tracked,
                    samples=args.samples, order_seed=7301,
                    binaries=[dict(name=path.name, sha256=sha256(path),
                                   go_version=run(["go", "version", "-m", path], ROOT).stdout.decode().splitlines()[1:])
                              for path in binaries], profiles=profiles)
    revisions = load(ROOT / "experiment/stages.json")
    for binary, revision in zip(metadata["binaries"][1:], revisions, strict=True):
        assert f"\tbuild\tvcs.revision={revision['commit']}" in binary["go_version"], binary
        assert "\tbuild\tvcs.modified=false" in binary["go_version"], binary
    write_json(output / "metadata.json", metadata)
    cases = load_cases(research)
    write_json(output / "cases.json", cases)
    units = go_unit_index(binaries[0], research)
    summary, measurements = [], []
    order = random.Random(metadata["order_seed"])
    warning_sets = {}

    for key, case in cases.items():
        observed = {}
        times = defaultdict(list)
        for sample in range(args.samples + 1):
            ordered = list(profiles)
            order.shuffle(ordered)
            for position, mode in enumerate(ordered, 1):
                stage, flags = profiles[mode]
                argv = [binaries[stage], "--ask", case["query"], "--top", "50", "--format", "json",
                        *flags, str(CORPUS)]
                start = time.perf_counter_ns()
                got = run(argv, research)
                elapsed = (time.perf_counter_ns() - start) / 1e6
                payload = got.stdout, got.stderr
                if sample == 0:
                    observed[mode] = payload
                    (raw / f"{key}-{mode}.stdout").write_bytes(got.stdout)
                    (raw / f"{key}-{mode}.stderr").write_bytes(got.stderr)
                    if mode == "original" and case["suite"] == "fixed":
                        for suffix, content in zip(("stdout", "stderr"), payload, strict=True):
                            assert content == (reference / f"{key}-rank-50.{suffix}").read_bytes(), (key, suffix)
                else:
                    assert payload == observed[mode], (key, mode, sample)
                    times[mode].append(elapsed)
                    measurements.append(dict(query=key, profile=mode, sample=sample, order=position,
                                             duration_ms=f"{elapsed:.3f}"))
        for mode in profiles:
            stdout, stderr = observed[mode]
            warnings = stderr.decode().splitlines()
            warning_sets.setdefault(mode, warnings)
            assert warning_sets[mode] == warnings, (key, mode)
            summary.append(dict(query=key, kind=case["kind"], suite=case["suite"], profile=mode,
                                stdout_bytes=len(stdout), stderr_bytes=len(stderr),
                                duration_median_ms=f"{statistics.median(times[mode]):.3f}",
                                **score(json.loads(stdout) or [], case["anchors"], units)))
        print(f"{key}: {len(profiles)} profiles complete", flush=True)

    write_tsv(output / "summary.tsv", summary)
    write_tsv(output / "samples.tsv", measurements)
    write_json(output / "warnings.json", warning_sets)
    if not retained_docs:
        print("Replaying unchanged CLI commands against the stage executables", flush=True)
        write_json(output / "regressions.json", replay(research, binaries[1:], reference))
        write_json(output / "option-checks.json", check_options(research, binaries[-1], reference, output))
    assert not run(["git", "status", "--porcelain"], research / CORPUS).stdout


if __name__ == "__main__":
    main()
