#!/usr/bin/env node
// Baseline benchmark for launcher search paths.
//
// Loads the REAL repo sources and scores a deterministic synthetic corpus:
// - Modals/DankLauncherV2/Scorer.js (verbatim, pragma stripped)
// - Pure fns extracted from Services/AppSearchService.qml by name
//   (tokenize, wordBoundaryMatch, levenshteinDistance, fuzzyMatchScore).
//   Extraction fails loudly if those functions are renamed.
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

function loadScorer() {
    const src = readFileSync(path.join(QS, "Modals/DankLauncherV2/Scorer.js"), "utf8")
        .split("\n")
        .filter((l) => !/^\s*\.pragma/.test(l) && !/^\s*\.import/.test(l))
        .join("\n");
    const box = {};
    vm.runInNewContext(
        src + "\nthis.__api = { score, scoreItems, fuzzyScore, calculateTextScore, tokenize, hasWordBoundaryMatch, levenshteinDistance };",
        box,
        { filename: "Scorer.js" },
    );
    return box.__api;
}

function extractQmlFunction(src, file, name) {
    const marker = `function ${name}(`;
    const start = src.indexOf(marker);
    if (start === -1) throw new Error(`${file}: function ${name}() not found, update bench`);
    const open = src.indexOf("{", start);
    let depth = 0, i = open, str = null, line = false, block = false;
    for (; i < src.length; i++) {
        const c = src[i], n = src[i + 1];
        if (str) {
            if (c === "\\") { i++; continue; }
            if (c === str) str = null;
            continue;
        }
        if (line) { if (c === "\n") line = false; continue; }
        if (block) { if (c === "*" && n === "/") { block = false; i++; } continue; }
        if (c === '"' || c === "'" || c === "`") { str = c; continue; }
        if (c === "/" && n === "/") { line = true; i++; continue; }
        if (c === "/" && n === "*") { block = true; i++; continue; }
        if (c === "{") depth++;
        if (c === "}") { depth--; if (depth === 0) break; }
    }
    if (depth !== 0) throw new Error(`${file}: unbalanced braces in ${name}(), update bench`);
    return src.slice(start, i + 1);
}

function loadAppServicePure() {
    const file = "Services/AppSearchService.qml";
    const src = readFileSync(path.join(QS, file), "utf8");
    const names = ["tokenize", "wordBoundaryMatch", "levenshteinDistance", "fuzzyMatchScore"];
    const combined = names.map((n) => extractQmlFunction(src, file, n)).join("\n");
    const box = {};
    vm.runInNewContext(combined + "\nthis.__api = { tokenize, wordBoundaryMatch, levenshteinDistance, fuzzyMatchScore };", box, { filename: "AppSearchService-pure" });
    return box.__api;
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
    const appSvc = loadAppServicePure();
    const items = buildCorpus(CORPUS);
    const longName = items[0].name + " " + items[0].subtitle;

    const cases = [
        ["scorer/scoreItems empty query", () => scorer.scoreItems(items, "", frecencyStub)],
        ["scorer/scoreItems prefix 'term'", () => scorer.scoreItems(items, "term", frecencyStub)],
        ["scorer/scoreItems multiword 'web brow'", () => scorer.scoreItems(items, "web brow", frecencyStub)],
        ["scorer/scoreItems nomatch+fuzzy 'zxqv'", () => scorer.scoreItems(items, "zxqv", frecencyStub)],
        ["appsvc/fuzzyMatchScore worst-case", () => appSvc.fuzzyMatchScore(longName, "zxqv")],
        ["appsvc/fuzzyMatchScore hit", () => appSvc.fuzzyMatchScore("Terminal Emulator", "term")],
        ["appsvc/wordBoundaryMatch", () => appSvc.wordBoundaryMatch("Firefox Web Browser", "web brow")],
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
