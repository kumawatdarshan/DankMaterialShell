# Clipboard Socket API

Reference for the `clipboard.*` methods on the `dms` unix-socket server
(see `dms ipc` for the shell-surface IPC, documented in `IPC.md`).
All searches are newest-first.

## `clipboard.search`

Paginated history search. Preferred over fetching full history.

Params:

| Param | Type | Description |
|---|---|---|
| `query` | string | Case-insensitive substring over entry previews. Empty matches all. |
| `entryType` | string | `all`, `text` (short text only), `long_text`, or `image`. Empty means `all`. Unknown values match nothing. |
| `pinned` | bool | Omit for both, `true` for saved entries only, `false` for history only. |
| `limit` | number | Max entries per page, default 50, capped at 500. |
| `beforeId` | number | Cursor: pass the lowest entry `id` of the previous page to fetch older rows. Omit (or 0) for the newest page. The cursor seeks to that key and walks backwards, so concurrent inserts and deletes cause no gaps or duplicates. |

Result:

```json
{
  "entries": [{"id": 42, "preview": "...", "mimeType": "text/plain", "isImage": false, "size": 12, "timestamp": "...", "hash": 1, "pinned": false}],
  "total": 137,
  "totalKnown": true,
  "hasMore": true
}
```

- `total` is exact only when `totalKnown` is `true`. That happens for plain
  history views: no query, no entry type filter, `pinned: false`. The count
  comes from a cached counter, no scan.
- Filtered searches report `total: -1` with `totalKnown: false`. Page with
  `beforeId` until `hasMore` is `false`; never do arithmetic on `-1`.
- At most `limit` entries are returned; when more exist the response carries
  `limit` entries with `hasMore: true`.

Example paging loop (page size 500):

```js
let beforeId = undefined;
for (;;) {
    const page = await call("clipboard.search", { limit: 500, beforeId });
    render(page.entries);
    if (!page.hasMore || page.entries.length === 0) break;
    beforeId = page.entries[page.entries.length - 1].id;
}
```

## `clipboard.deleteMatching`

Deletes unpinned entries matching a search without paging through them.
Backs "clear filtered" UIs that only hold the visible page.

Params: `query?`, `entryType?` (same semantics as `clipboard.search`).

Result: `{ "deleted": <number> }` — the exact count removed. Pinned
entries are never touched.

## Removed: `clipboard.getHistory`

Deleted. It returned the unbounded full history in one response.
Migrate to `clipboard.search`:

- Full dump: loop with no `query`/`entryType` filter as above.
- Saved entries: `clipboard.getPinnedEntries` (complete set, no paging needed).
