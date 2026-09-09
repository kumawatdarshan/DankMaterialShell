#!/usr/bin/env node
// Benchmark for launcher search paths.
//
// Loads the REAL repo sources and scores a deterministic synthetic corpus:
// - Modals/DankLauncherV2/Scorer.js (verbatim, pragma stripped)
// - Common/SearchUtils.js (verbatim, pragma stripped) for the filter gate
//
// "scorer/full" cases score the whole corpus (pre-gate control).
// "pipeline" cases mirror production since PR7: substring gate over the
// normalized index, then Scorer.scoreItems on the gated subset, top 10.
//
// Numbers are V8-relative (QML runs QJSEngine), so compare runs against each
// other, not against Go benchmarks or wall-clock budgets.
//
// Usage: node quickshell/bench/launcher-search.mjs [--corpus 800] [--iters 15]
import { readFileSync } from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const QS = path.resolve(HERE, "..");

const args = process.argv.slice(2);
function flag(name, def) {
    const i = args.indexOf(name);
    if (i === -1) return def;
    const v = Number(args[i + 1]);
    return Number.isFinite(v) && v > 0 ? v : def;
}
const CORPUS = flag("--corpus", 800);
const ITERS = flag("--iters", 15);
const WARMUP = 5;

function mulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

function loadLib(relpath, api, filename) {
    const src = readFileSync(path.join(QS, relpath), "utf8")
        .split("\n")
        .filter((l) => !/^\s*\.pragma/.test(l) && !/^\s*\.import/.test(l))
        .join("\n");
    const box = {};
    vm.runInNewContext(src + `\nthis.__api = { ${api} };`, box, { filename });
    return box.__api;
}

function loadScorer() {
    return loadLib(
        "Modals/DankLauncherV2/Scorer.js",
        "score, scoreItems, fuzzyScore, calculateTextScore, tokenize, hasWordBoundaryMatch, levenshteinDistance",
        "Scorer.js",
    );
}

function loadSearchUtils() {
    return loadLib(
        "Common/SearchUtils.js",
        "fold, tokenize, foldAndTokenize, buildNormalizedIndex, score",
        "SearchUtils.js",
    );
}

const BASE_NAMES = [
    "Firefox Web Browser", "Terminal", "Terminal Emulator", "Files", "Text Editor",
    "Code Editor", "Settings", "Calculator", "Music Player", "Video Player",
    "Image Viewer", "Document Viewer", "System Monitor", "Disk Manager",
    "Network Manager", "Password Manager", "Calendar", "Contacts", "Maps",
    "Weather", "Notes", "Mail Client", "Chat Client", "PDF Reader",
    "Archive Manager", "Screenshot Tool", "Color Picker", "Task Manager",
    "Webcam Viewer", "Audio Mixer", "Font Manager", "Backup Tool",
];
const GENERICS = ["Web Browser", "File Manager", "Text Editor", "System Settings", "Media Player", "Utility", ""];
const KEYWORDS = ["web", "net", "dev", "media", "office", "sys", "tool", "gtk", "qt", "flatpak", "gpu", "cloud", "edit", "view", "play"];

function buildCorpus(n) {
    const rnd = mulberry32(0xc11ab0);
    const items = [];
    for (let i = 0; i < n; i++) {
        const base = BASE_NAMES[Math.floor(rnd() * BASE_NAMES.length)];
        const variant = rnd();
        const name = variant < 0.15 ? `${base} Nightly` : variant < 0.25 ? `${base} (Flatpak)` : base;
        const generic = GENERICS[Math.floor(rnd() * GENERICS.length)];
        const kws = [];
        const nk = Math.floor(rnd() * 4);
        for (let k = 0; k < nk; k++) kws.push(KEYWORDS[Math.floor(rnd() * KEYWORDS.length)]);
        items.push({
            id: `org.example.app${i}.desktop`,
            type: "app",
            name,
            subtitle: `${generic} infrastructure component ${i}`,
            icon: "application-x-executable",
            iconType: "image",
            section: "apps",
            data: { genericName: generic },
            keywords: kws,
            actions: [],
            source: "system",
        });
    }
    return items;
}

function frecencyStub(item) {
    let h = 0;
    for (let i = 0; i < item.id.length; i++) h = (h * 31 + item.id.charCodeAt(i)) >>> 0;
    return { usageCount: h % 20 };
}

function measure(label, fn) {
    for (let i = 0; i < WARMUP; i++) fn();
    const ts = [];
    for (let i = 0; i < ITERS; i++) {
        const t0 = process.hrtime.bigint();
        fn();
        ts.push(Number(process.hrtime.bigint() - t0) / 1e6);
    }
    ts.sort((a, b) => a - b);
    const mean = ts.reduce((a, b) => a + b, 0) / ts.length;
    return { label, mean, median: ts[Math.floor(ts.length / 2)], p95: ts[Math.min(ts.length - 1, Math.floor(ts.length * 0.95))] };
}

function main() {
    const scorer = loadScorer();
    const SU = loadSearchUtils();
    const items = buildCorpus(CORPUS);
    const index = SU.buildNormalizedIndex(items, [
        "name",
        "subtitle",
        "keywords",
        (item) => item.data.genericName,
        "id",
    ]);

    // Production pipeline since PR7: cheap score gate, fuzzy top-up when
    // scarce, then ranked scoring on the candidate set, top 10.
    const pipeline = (query) => {
        const q = query.toLowerCase().trim();
        if (!q) return items.slice(0, 10);
        const qq = SU.foldAndTokenize(q);
        let gated = index.filter((e) => SU.score(e, qq) > 0).map((e) => e.item);
        if (gated.length < 10 && q.length >= 3) {
            const excluded = new Set(gated);
            const scored = [];
            for (const item of items) {
                if (excluded.has(item)) continue;
                const fs = scorer.fuzzyScore((item.name || "").toLowerCase(), q);
                if (fs > 0) scored.push({ item, s: fs });
            }
            scored.sort((a, b) => b.s - a.s);
            for (let j = 0; j < scored.length && gated.length < 10; j++) gated.push(scored[j].item);
        }
        return scorer.scoreItems(gated, query, frecencyStub).slice(0, 10);
    };

    const cases = [
        ["scorer/full empty query", () => scorer.scoreItems(items, "", frecencyStub)],
        ["scorer/full prefix 'term'", () => scorer.scoreItems(items, "term", frecencyStub)],
        ["scorer/full multiword 'web brow'", () => scorer.scoreItems(items, "web brow", frecencyStub)],
        ["scorer/full nomatch+fuzzy 'zxqv'", () => scorer.scoreItems(items, "zxqv", frecencyStub)],
        ["pipeline prefix 'term'", () => pipeline("term")],
        ["pipeline multiword 'web brow'", () => pipeline("web brow")],
        ["pipeline nomatch 'zxqv'", () => pipeline("zxqv")],
        ["pipeline typo 'termnal'", () => pipeline("termnal")],
    ];

    const rows = cases.map(([label, fn]) => ({ ...measure(label, fn), corpus: CORPUS }));
    const w = (s, n) => String(s).padEnd(n);
    console.log(`launcher-search bench | node ${process.version} | corpus=${CORPUS} iters=${ITERS} warmup=${WARMUP}`);
    console.log(`${w("case", 38)} ${w("mean ms", 10)} ${w("median ms", 11)} ${w("p95 ms", 10)} ops/s`);
    for (const r of rows) {
        console.log(`${w(r.label, 38)} ${w(r.mean.toFixed(3), 10)} ${w(r.median.toFixed(3), 11)} ${w(r.p95.toFixed(3), 10)} ${(1000 / r.mean).toFixed(1)}`);
    }
}

main();
