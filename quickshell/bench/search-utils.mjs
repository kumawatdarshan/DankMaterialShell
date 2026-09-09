#!/usr/bin/env node
// Correctness + speed test for quickshell/Common/SearchUtils.js (real file).
// Usage: node quickshell/bench/search-utils.mjs [--corpus 800] [--iters 20]
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
const ITERS = flag("--iters", 20);
const WARMUP = 5;

let failures = 0;
function check(name, cond) {
    if (!cond) {
        failures++;
        console.error(`FAIL: ${name}`);
    }
}

function loadSearchUtils() {
    const src = readFileSync(path.join(QS, "Common/SearchUtils.js"), "utf8")
        .split("\n")
        .filter((l) => !/^\s*\.pragma/.test(l) && !/^\s*\.import/.test(l))
        .join("\n");
    const box = {};
    vm.runInNewContext(
        src + "\nthis.__api = { fold, tokenize, foldAndTokenize, buildNormalizedIndex, score };",
        box,
        { filename: "SearchUtils.js" },
    );
    return box.__api;
}

function mulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

const SU = loadSearchUtils();

// Correctness
check("fold null", SU.fold(null) === "");
check("fold case", SU.fold("TeRm") === "term");
check("tokenize splits", JSON.stringify(SU.tokenize("web-brow_test x")) === JSON.stringify(["web", "brow", "test", "x"]));
check("foldAndTokenize empty", SU.foldAndTokenize("  ").tokens.length === 0);

const items = [
    { name: "Terminal", comment: "Emulator", keywords: ["shell", "term"] },
    { name: "Terminal Emulator", comment: "System tool", keywords: [] },
    { name: "Firefox Web Browser", comment: "Browse the web", keywords: ["net"] },
    { name: "Café Notes", comment: "Take notes", keywords: [] },
];
const index = SU.buildNormalizedIndex(items, ["name", "comment", "keywords"]);
check("index shape", index.length === 4 && index[0].folded[0] === "terminal" && index[0].folded[2] === "shell");

const q = (s) => SU.foldAndTokenize(s);
check("empty query neutral", SU.score(index[0], q("")) === 1);
check("exact beats prefix", SU.score(index[0], q("terminal")) > SU.score(index[1], q("terminal")));
check("prefix beats substring", SU.score(index[1], q("term")) > SU.score(index[0], q("erm")));
check("word boundary", SU.score(index[2], q("web brow")) > 0);
check("keyword match", SU.score(index[0], q("shell")) > 0);
check("no match", SU.score(index[0], q("zxqv")) === 0);
check("unicode", SU.score(index[3], q("café")) > 0);
check("multiword all-tokens", SU.score(index[2], q("firefox web")) > 0);
check("multiword missing token", SU.score(index[2], q("firefox zzz")) === 0);

const filtered = index.filter((e) => SU.score(e, q("term")) > 0);
check("filter pattern", filtered.length === 2);

if (failures > 0) {
    console.error(`${failures} correctness check(s) failed`);
    process.exit(1);
}
console.log(`correctness: all checks passed (${index.length} indexed items)`);

// Speed over a synthetic corpus shaped like transformed launcher items
const NAMES = ["Firefox Web Browser", "Terminal", "Terminal Emulator", "Files", "Text Editor", "Settings", "Music Player", "Calendar", "Mail Client", "Code Editor"];
function buildCorpus(n) {
    const rnd = mulberry32(0x5eed);
    const out = [];
    for (let i = 0; i < n; i++) {
        const name = NAMES[Math.floor(rnd() * NAMES.length)];
        out.push({ name, subtitle: `generic component ${i}`, keywords: ["sys", "tool"] });
    }
    return out;
}
const corpusIndex = SU.buildNormalizedIndex(buildCorpus(CORPUS), ["name", "subtitle", "keywords"]);
const queries = ["", "term", "web brow", "zxqv"].map(q);

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

const rows = queries.map((qq, i) => measure(
    ["empty", "prefix 'term'", "multiword", "nomatch"][i],
    () => {
        let n = 0;
        for (const e of corpusIndex) if (SU.score(e, qq) > 0) n++;
        return n;
    },
));
const w = (s, n) => String(s).padEnd(n);
console.log(`search-utils bench | node ${process.version} | corpus=${CORPUS} iters=${ITERS} warmup=${WARMUP}`);
console.log(`${w("case", 16)} ${w("mean ms", 10)} ${w("median ms", 11)} ${w("p95 ms", 10)} ops/s`);
for (const r of rows) {
    console.log(`${w(r.label, 16)} ${w(r.mean.toFixed(3), 10)} ${w(r.median.toFixed(3), 11)} ${w(r.p95.toFixed(3), 10)} ${(1000 / r.mean).toFixed(1)}`);
}
