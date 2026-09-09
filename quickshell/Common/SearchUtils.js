.pragma library

// Shared, dependency-free search primitives for filter-only passes and
// simple lists (launcher plugin items, app picker, mux sessions, peer and
// device lists, keybind cheatsheet).
//
// Import relatively, e.g. from Modals/DankLauncherV2:
//     import "../../Common/SearchUtils.js" as SearchUtils
// Third-party plugins live outside this tree, so vendor these functions
// (or the normalized-index pattern below) instead of importing them.
//
// Scoring authority note: ranked and fuzzy scoring lives in
// Modals/DankLauncherV2/Scorer.js. score() here is the cheap tier gate for
// filter-only passes: exact > prefix > word-boundary > substring, no
// Levenshtein. Anything needing typo tolerance must use Scorer.js.
//
// All matching is case-insensitive via pre-folded fields. Never call
// toLowerCase() per item per keystroke: build the index once with
// buildNormalizedIndex() and score against it.

function fold(text) {
    if (text === null || text === undefined) {
        return "";
    }
    return String(text).toLowerCase();
}

function tokenize(text) {
    return fold(text).trim().split(/[\s\-_]+/).filter(function (w) {
        return w.length > 0;
    });
}

// foldAndTokenize normalizes the query once per keystroke. The result feeds
// score(): { folded: "web brow", tokens: ["web", "brow"] }.
function foldAndTokenize(text) {
    var folded = fold(text).trim();
    return {
        folded: folded,
        tokens: folded.length === 0 ? [] : folded.split(/[\s\-_]+/).filter(function (w) {
            return w.length > 0;
        })
    };
}

// buildNormalizedIndex precomputes folded fields once per collection change.
// fields is an array of property names or extractor functions returning a
// string or string array. The first field is the primary (name-like) field.
// Returns [{ item, folded: [primary, ...rest] }].
function buildNormalizedIndex(items, fields) {
    var extractors = fields.map(function (f) {
        if (typeof f === "function") {
            return f;
        }
        return function (item) {
            return item ? item[f] : "";
        };
    });
    return items.map(function (item) {
        var folded = [];
        for (var i = 0; i < extractors.length; i++) {
            var value = extractors[i](item);
            if (Array.isArray(value)) {
                for (var j = 0; j < value.length; j++) {
                    folded.push(fold(value[j]));
                }
            } else {
                folded.push(fold(value));
            }
        }
        return {
            item: item,
            folded: folded
        };
    });
}

function hasWordBoundary(textFolded, queryTokens) {
    if (queryTokens.length === 0) {
        return false;
    }
    var textWords = textFolded.split(/[\s\-_]+/);
    if (queryTokens.length > textWords.length) {
        return false;
    }
    for (var i = 0; i <= textWords.length - queryTokens.length; i++) {
        var allMatch = true;
        for (var j = 0; j < queryTokens.length; j++) {
            if (textWords[i + j].indexOf(queryTokens[j]) !== 0) {
                allMatch = false;
                break;
            }
        }
        if (allMatch) {
            return true;
        }
    }
    return false;
}

// score ranks one normalized entry against a foldAndTokenize() query.
// Empty queries match everything with a neutral score. Returns 0 for no
// match, so callers filter with: index.filter(e => SearchUtils.score(e, q) > 0)
function score(entry, query) {
    var q = typeof query === "string" ? foldAndTokenize(query) : query;
    if (!q || q.folded.length === 0) {
        return 1;
    }
    var fields = entry.folded;
    if (!fields || fields.length === 0) {
        return 0;
    }
    var primary = fields[0];
    if (primary === q.folded) {
        return 10000;
    }
    if (primary.indexOf(q.folded) === 0) {
        return 5000;
    }
    if (hasWordBoundary(primary, q.tokens)) {
        return 3000;
    }
    if (primary.indexOf(q.folded) !== -1) {
        return 500;
    }
    for (var i = 1; i < fields.length; i++) {
        if (fields[i].indexOf(q.folded) === 0) {
            return 800;
        }
        if (fields[i].indexOf(q.folded) !== -1) {
            return 400;
        }
    }
    if (q.tokens.length > 1) {
        var haystack = fields.join(" ");
        for (var t = 0; t < q.tokens.length; t++) {
            if (haystack.indexOf(q.tokens[t]) === -1) {
                return 0;
            }
        }
        return 300;
    }
    return 0;
}
