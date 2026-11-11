# Sources for collecting

`osintCollect` reads open sources into a file of items; `osintActivity` adds the file up and
drafts case entries from it. Which sources exist is up to the user: three are built in, and
any JSON API can be added in a sources file, with the user's own key.

These tools exist on the local server only. They act for the user, from the user's machine,
with the user's sources file and keys; a hosted server has none of these and does not offer
them.

## Built-in sources

| Source id | `target` | What it reads | Key |
|---|---|---|---|
| `telegram_channel` | channel name, `@name` or a `t.me` link | The posts of a public channel as Telegram's own preview (`t.me/s/<name>`) shows them without a login: date, text, links, views. Newest first, paged back until `limit` or `since` | none |
| `web_page` | a page URL | What a page says about itself: title, description, picture, author (its `og:` and `meta` tags). One item | none |
| `rss` | a feed URL | The entries of an RSS or Atom feed: date, title, summary, author, link, categories | none |

A private channel, a profile behind a login and a site that refuses automated requests are
not read, and the tools do not try to get past them.

## Your own sources: `osint-sources.yaml`

Any API that answers with JSON can be a source. Describe it in `osint-sources.yaml` in the
working directory (or name another file with `sourcesPath`):

```yaml
sources:
  - id: news                       # the id you pass to osintCollect
    kind: http_json
    title: News search
    url: "https://newsapi.example/v2/everything?q={target}&page={page}&pageSize=50"
    headers:
      X-Api-Key: "${OSINT_NEWS_KEY}"     # the key stays in the environment
    items: articles                # where the list is in the reply
    fields:                        # where each part of an item is, inside one element
      id: url
      url: url
      date: publishedAt
      title: title
      text: description
      author: source.name
    source_kind: media             # the `kind` of the case source entry (see the case format)
    reliability: C                 # a starting grade for it
```

| Key | Meaning |
|---|---|
| `id` | Lower-case letters, digits, `_`, `-`. Not one of the built-in ids. |
| `kind` | `http_json`. |
| `url` | An http(s) URL. `{target}` is replaced with the `target` argument (URL-escaped), `{page}` with the page number, `{cursor}` with the cursor from the previous reply. Without `{page}` or `{cursor}` one request is made. |
| `headers` | Request headers. |
| `items` | Dotted path to the list in the reply (`data.items`), or `"."` when the reply itself is the list. Required. |
| `fields` | Dotted paths, inside one element, to `id`, `url`, `date`, `title`, `text`, `author`. All optional; an RFC 3339 `date` lets `since` and the activity summary work. |
| `metrics` | Numbers to read from each element, as `name: path` — `metrics: {stars: stargazers_count, forks: forks_count}`. They are summed and ranked by `osintActivity` and recorded by `osintActivityRecord`. |
| `cursor` | Dotted path, in the reply, to the cursor for the next page; needs `{cursor}` in the url. Paging stops when it is empty. |
| `first_page` | The first value of `{page}` (default 1). |
| `source_kind`, `reliability` | Carried into the draft of the case source entry. |

Paging stops at `limit` items, at an empty page, at a page with nothing new, or after 300
requests.

### Keys

A key is never written in the sources file. Write `${OSINT_NAME}` in a header or in the url
and set the environment variable `OSINT_NAME` — in the shell that starts the host, or in the
`.env` file of the working directory, which the server loads.

- Only variables whose names start with `OSINT_` are read. A sources file cannot name any
  other variable of the process, so it cannot send a cloud credential or the Simulator token
  to its URL.
- A source that sends a key must use `https`.
- A redirect to another host is not followed, so a key never reaches a host the file does
  not name.
- `osintSources` shows which variables each source needs and whether they are set, never
  their values. Errors name the host, never the URL.

Review a sources file before using it, as you would a script: it decides where requests go.

## Using them

```
osintSources()
→ telegram_channel, web_page, rss, and the sources of osint-sources.yaml with "ready": true|false

osintCollect(source="telegram_channel", target="some_channel", outPath="cases/acme/channel.items.json",
             limit=1000, since="2025-01-01")
→ { "file": "…/channel.items.json", "items": 1000, "first": "2025-11-14", "last": "2026-11-10", "truncated": true }

osintActivity(itemsPath="cases/acme/channel.items.json", subject="channel", top=8)
→ items per month / weekday / hour, the spread of views with the highest posts,
  the most linked sites, mentioned accounts and tags, the sites the items are published on
  (when there is more than one), and a draft of case entries
```

`osintCollect` returns counts and the file name, and `osintActivity` returns counts and
identifiers. Neither returns the text of an item: what a source says is data, written by
whoever wrote it, and is read from the file only when the investigation needs it.

### From a collection to the case

With `subject` — the id, in the case, of the entity the collection is about — `osintActivity`
adds `caseDraft`: a source entry for the collection, and an organization with a `mentions`
relation for each of the most linked sites.

```yaml
sources:
  - {id: s-telegram-channel-some-channel, title: "telegram_channel some_channel: 1000 items, 2025-11-14 to 2026-11-10",
     kind: social, reliability: B, retrieved: 2026-11-10, url: "https://t.me/s/some_channel"}
entities:
  - {id: site-publisher-example, type: organization, title: "publisher.example", attrs: {website: "https://publisher.example"},
     sources: [s-telegram-channel-some-channel], credibility: 1}
relations:
  - {from: channel, to: site-publisher-example, type: mentions, sources: [s-telegram-channel-some-channel], credibility: 1,
     note: "Linked from 42 of 1000 items."}
```

It is a draft. Give the organizations their real names, drop what does not belong, and
remember what a link count does and does not show: that the items pointed there, not why.
Sites that host everyone's pages (social networks, video and document hosts) are ranked but
not drafted as organizations.

### From a collection to accounts

`osintActivityRecord` writes a collection onto an actor as accounts, so that Simulator's own
charts and triggers work on it:

```
osintActivityRecord(itemsPath="cases/acme/channel.items.json", actorId="<the channel's actor>", bucket="month")
→ accounts "Activity · items" (984) and "Activity · views" (3 108 970), 12 entries each
```

- One account for the number of items and one for each metric the source gives, in the
  currency `units`.
- One entry per day, week (default) or month, dated at the start of that period, so the
  account's turnover by period is the activity of that period. `createChart` draws it: use
  the chart type `line` or `stackedBar` with the account's `credit` side — a `bar` chart shows
  one total, not the periods — and place the chart beside the graph, not among its nodes,
  where it would cover the links.
- `label` names the accounts (`<label> · items`, default `Activity`). Give one — `Press`,
  `GitHub` — when several collections are recorded on the same actor.
- Entries are found again by their ref: recording the same collection again, or a later one
  that overlaps it, adds only what is new. This is how a twin is kept current — collect
  again, record again.
- A period still running when the collection was made is left out, and so is the earliest
  period of a collection cut at its limit: both would be recorded short.

It writes to the workspace; confirm the actor with the user first.

## Limits

| | |
|---|---|
| Items per collection | 200 by default, 5 000 at most |
| Requests per collection | 300, with a pause between them |
| One reply | 5 MB |
| Text kept per item | 2 000 characters |
| Time per collection | 10 minutes |

## What to keep in mind

- Collect about a real person only what the investigation's question needs, from sources you
  may lawfully use, and within what the person agreed to or the law allows. The format
  reference and the `simulator-osint` skill say more.
- A source's terms of use are the user's to respect: the tools read what is open, at a slow
  pace, and identify themselves in the `User-Agent`.
- Counts cover what was collected. `truncated: true` means the source had more than the limit.
- A search API returns what it thinks matches, which is not always what was asked: a search
  for a company's name can return pages about similarly spelled words. Before trusting a
  count from a search source, look at `publishedOn` and at a sample of titles in the file.
