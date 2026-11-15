import { OWID_CHART_URL_PREFIX } from "./constants";
import { applyChartSourceToDescription } from "./utils";
import { IframeImporterFormItem, IframeInfo, IframeNameHints, TaskProcess, TaskProcessStatusEnum } from "./types";

// Mirrors the OWIDPopup gadget's accepted `data-chart` values and the server's NormalizeIframeChartPath.
export const IFRAME_CHART_PATH_REGEX = /^(grapher|explorers)\/[a-z0-9-]+?($|\?)/;

// Characters/sequences that would break out of the {{OWID}} template parameters or the wikitext line.
// Also "<!--" (comments out the rest of the template), "~~~" (becomes a signature on save) and any control character.
// eslint-disable-next-line no-control-regex
const FORBIDDEN_CHART_PATH_REGEX = /[|\u0000-\u001f\u007f-\u009f]|\{\{|\}\}|\[\[|\]\]|<!--|~~~/;

const OWID_URL_PREFIX_REGEX = /^https?:\/\/(www\.)?ourworldindata\.org/;

/**
 * Normalizes a full OWID URL or a bare path to the chart path used by {{OWID}}
 * (e.g. `explorers/minerals?tab=map`). The query string is kept byte-for-byte.
 * Returns null when the input cannot be imported.
 */
export function normalizeIframeChartPath(input: string): string | null {
  let path = input.trim();
  path = path.replace(OWID_URL_PREFIX_REGEX, "");
  path = path.replace(/^\/+/, "");
  path = stripOverlayParam(path);
  if (!IFRAME_CHART_PATH_REGEX.test(path) || FORBIDDEN_CHART_PATH_REGEX.test(path)) {
    return null;
  }
  return path;
}

// Longest `?chart=` value prefilled into the form; anything longer leaves the form blank.
export const CHART_PARAM_MAX_LENGTH = 2048;

// `chart=` as a whole parameter name (at the start of the query or after `&`).
const CHART_PARAM_REGEX = /(?:^|&)chart=/;

// eslint-disable-next-line no-control-regex
const CONTROL_CHARS_REGEX = /[\u0000-\u001f\u007f-\u009f]/g;

/**
 * The chart URL or path to prefill from a page's query string (`?chart=...`), or "" for a blank form.
 * Everything after the first `chart=` is taken, so an unencoded nested query
 * (`?chart=https://ourworldindata.org/grapher/x?tab=map&time=2020`) stays whole. A value whose first `&`-segment has no
 * literal `/` was percent-encoded as a whole (encodeURIComponent turns `/` into %2F): it ends at the
 * next `&` and is decoded. Control characters are dropped and the value is trimmed; malformed escapes
 * and values over CHART_PARAM_MAX_LENGTH give "". The result is untrusted text: it is only shown as the
 * Chart URL value and looked up when normalizeIframeChartPath accepts it.
 */
export function readChartParam(search: string): string {
  const query = search.replace(/^\?/, "");
  const match = CHART_PARAM_REGEX.exec(query);
  if (!match) return "";
  let value = query.slice(match.index + match[0].length);
  if (value.length > CHART_PARAM_MAX_LENGTH * 3) return "";
  const firstSegment = value.split("&", 1)[0];
  if (!firstSegment.includes("/")) {
    value = firstSegment;
    try {
      value = decodeURIComponent(value);
    } catch {
      return "";
    }
  }
  value = value.replace(CONTROL_CHARS_REGEX, "").trim();
  return value.length > CHART_PARAM_MAX_LENGTH ? "" : value;
}

/**
 * Removes every `overlay` query parameter (`overlay=download-vis` opens OWID's download modal on load,
 * for readers too); mirrors the server. Everything else is kept byte-for-byte; a bare `?` is dropped.
 */
export function stripOverlayParam(path: string): string {
  const queryStart = path.indexOf("?");
  if (queryStart === -1) return path;
  const hashStart = path.indexOf("#", queryStart);
  const queryEnd = hashStart === -1 ? path.length : hashStart;
  const params = path.slice(queryStart + 1, queryEnd).split("&");
  const kept = params.filter(param => param.split("=", 1)[0] !== "overlay");
  if (kept.length === params.length) return path;
  const query = kept.join("&");
  return query ? path.slice(0, queryStart + 1) + query + path.slice(queryEnd) : path.slice(0, queryStart) + path.slice(queryEnd);
}

/**
 * Replaces a single-year `time=<year>` query parameter with `time=latest`. Ranges, dates, `latest`,
 * `earliest` and URLs without a `time` parameter are returned unchanged, as is everything else
 * (parameter order, encoding, fragment). Works on full URLs and bare paths; surrounding whitespace is trimmed.
 */
export function applyLatestTime(input: string): string {
  const urlOrPath = input.trim();
  const queryStart = urlOrPath.indexOf("?");
  if (queryStart === -1) {
    return urlOrPath;
  }
  const hashStart = urlOrPath.indexOf("#", queryStart);
  const queryEnd = hashStart === -1 ? urlOrPath.length : hashStart;
  const query = urlOrPath.slice(queryStart + 1, queryEnd)
    .split("&")
    .map(param => /^time=-?\d+$/.test(param) ? "time=latest" : param)
    .join("&");
  return urlOrPath.slice(0, queryStart + 1) + query + urlOrPath.slice(queryEnd);
}

/** The URL an item is imported with: `time=<year>` becomes `time=latest` while the item's option is on. */
export function effectiveIframeUrl(item: Pick<IframeImporterFormItem, "url" | "useLatestTime">): string {
  return item.useLatestTime ? applyLatestTime(item.url) : item.url;
}

export function iframeChartUrl(path: string) {
  return `${OWID_CHART_URL_PREFIX}/${path}`;
}

/** The text a source lookup puts in place of `$SOURCE` (the default credit when the chart has no source). */
export function chartSourceCredit(source?: string) {
  return applyChartSourceToDescription("$SOURCE", source);
}

/**
 * Applies a lookup to a description the user may have edited: occurrences of the previous chart's
 * source text (when given and present) become the new chart's source, then any remaining `$SOURCE`
 * is filled in. Everything else in the text is left alone. `template` is the text with the
 * previous source turned back into `$SOURCE`, so undoing this lookup on a later URL change works.
 */
export function applySourceToEditedDescription(description: string, newSource?: string, previousCredit?: string) {
  const credit = chartSourceCredit(newSource);
  const swap = !!previousCredit && previousCredit !== credit && description.includes(previousCredit);
  const template = swap ? description.replaceAll(previousCredit, () => "$SOURCE") : description;
  return { template, description: applyChartSourceToDescription(template, newSource) };
}

/** Appends `[[Category:..]]` lines exactly like the map importer does. */
export function buildDescriptionWithCategories(description: string, categories: string[]) {
  let finalDescription = description.trim();
  if (categories.length > 0) {
    finalDescription += `\n${categories.map(category => `[[Category:${category}]]`).join("\n")}`;
  }
  return finalDescription;
}

/** Joins per-item {{OWID}} blocks with a blank line, skipping items without wikitext. */
export function joinWikitextBlocks(blocks: (string | undefined)[]) {
  return blocks
    .map(block => (block ?? "").replace(/\s+$/, ""))
    .filter(block => block.length > 0)
    .join("\n\n");
}

/** A task that could not be created on submit, kept so it can be shown and resubmitted. */
export interface IframeSubmitFailure {
  id: string
  url: string
  error: string
}

/** A state update derived from the latest item, so async results never overwrite what the user typed meanwhile. */
export type IframeItemUpdate = (prev: IframeImporterFormItem) => IframeImporterFormItem;

/**
 * The item after the user edits the URL: the previous lookup (and the source it put in the description,
 * unless the user edited the description since) is invalidated. `previousSource` is the old chart's source
 * text when the description was edited, so the next successful lookup can swap it for the new one.
 */
export function editIframeUrl(prev: IframeImporterFormItem, url: string): { item: IframeImporterFormItem, previousSource?: string } {
  const { sourceApplied } = prev;
  const untouched = !!sourceApplied && prev.description === sourceApplied.applied;
  const description = untouched ? sourceApplied.template : prev.description;
  const previousSource = sourceApplied && !untouched && prev.info ? chartSourceCredit(prev.info.source) : undefined;
  return {
    item: { ...prev, url, description, sourceApplied: undefined, linkVerified: false, canImport: false, info: null },
    previousSource,
  };
}

/** A lookup result is only applied to an item whose URL is still the one that was looked up. */
function isCurrentLookup(prev: IframeImporterFormItem, requestedUrl: string) {
  return prev.url === requestedUrl && !prev.linkVerified;
}

/** Applies a successful lookup to the latest item; returns `prev` itself when the URL changed meanwhile. */
export function applyIframeLookupInfo(prev: IframeImporterFormItem, requestedUrl: string, info: IframeInfo, previousSource?: string): IframeImporterFormItem {
  if (!isCurrentLookup(prev, requestedUrl)) {
    return prev;
  }
  const { template, description } = applySourceToEditedDescription(prev.description, info.source, previousSource);
  return { ...prev, description, sourceApplied: { template, applied: description }, info, linkVerified: true, canImport: true };
}

/** Marks the looked-up URL as not importable; returns `prev` itself when the URL changed meanwhile. */
export function applyIframeLookupFailure(prev: IframeImporterFormItem, requestedUrl: string): IframeImporterFormItem {
  if (!isCurrentLookup(prev, requestedUrl)) {
    return prev;
  }
  return { ...prev, info: null, canImport: false, linkVerified: true };
}

/** The Commons file name an iframe task ended up with (the task's filename is only the `$NAME_$TAB.svg` template). */
export function iframeFinalFileName(processes: Pick<TaskProcess, "filename" | "status">[]): string | undefined {
  const done = processes.find(p => p.filename && p.status !== TaskProcessStatusEnum.Failed);
  return done?.filename.replace(/^File:/, "");
}

// What Commons turns into "-" on upload (MediaWiki's wfStripIllegalFilenameChars): characters not legal in
// titles (# < > [ ] | { } and control characters) plus `:`, `/` and `\`.
// eslint-disable-next-line no-control-regex
const COMMONS_ILLEGAL_FILE_CHARS = /[#<>[\]|{}:/\\\u0000-\u001f\u007f]/g;

/**
 * A file name as Commons stores it (mirrors the server's CommonsFileName): characters an upload cannot use
 * become `-`, `_` becomes a space, runs of spaces collapse, the name is trimmed and its first letter uppercased.
 */
export function commonsFileName(name: string): string {
  const cleaned = name.trim().replace(/^File:/, "")
    .replace(COMMONS_ILLEGAL_FILE_CHARS, "-")
    .replaceAll("_", " ")
    .replace(/ {2,}/g, " ")
    .trim();
  if (!cleaned) return "";
  const first = String.fromCodePoint(cleaned.codePointAt(0)!);
  return first.toUpperCase() + cleaned.slice(first.length);
}

/**
 * The Commons file name an import's file name template resolves to with a lookup of `path`
 * (mirrors the server's ProvisionalIframeFileName). Undefined when the template needs a value
 * the lookup does not have ($NAME without a download name, $TITLE/$SOURCE/$TAB without a lookup).
 */
export function resolveIframeFileName(template: string, path: string, info?: Pick<IframeInfo, "path" | "downloadName" | "title" | "source" | "tab"> | null): string | undefined {
  const lookedUp = !!info && info.path === path;
  const name = lookedUp ? (info.downloadName ?? "").replace(/\.svg$/i, "") : "";
  if (template.includes("$NAME") && !name) return undefined;
  if (!lookedUp && (template.includes("$TITLE") || template.includes("$SOURCE") || template.includes("$TAB"))) return undefined;
  let value = resolveIframeTab(template, lookedUp ? info.tab : "");
  if (lookedUp && info.source) value = value.replaceAll("$SOURCE", () => info.source);
  value = value.replaceAll("$URL", () => iframeChartUrl(path)).replaceAll("$NAME", () => name);
  if (lookedUp && info.title) value = value.replaceAll("$TITLE", () => info.title);
  value = value.replaceAll("$SOURCE", "Our World in Data");
  return commonsFileName(value) || undefined;
}

/**
 * Substitutes $TAB (mirrors the server's resolveIframeVars). An unknown tab is removed with the
 * brackets around it ("($TAB)", "[$TAB]") and the run of spaces, "_" and "-" before it, or after
 * it when it starts the line: "$NAME - $TAB" and "$TAB - $NAME" both give "$NAME".
 */
export function resolveIframeTab(value: string, tab?: string): string {
  if (tab) return value.replaceAll("$TAB", () => tab);
  return value
    .replace(/^(?:\(\$TAB\)|\[\$TAB\]|\$TAB)[ _-]*/gm, "")
    .replace(/[ _-]*(?:\(\$TAB\)|\[\$TAB\]|\$TAB)/g, "");
}

/** The lookup values sent with a new task for its provisional file name; only when they are for the URL being imported. */
export function iframeNameHints(item: Pick<IframeImporterFormItem, "url" | "useLatestTime" | "info">): IframeNameHints | undefined {
  const { info } = item;
  if (!info || info.path !== normalizeIframeChartPath(effectiveIframeUrl(item))) return undefined;
  return { path: info.path, downloadName: info.downloadName ?? "", title: info.title, source: info.source, tab: info.tab };
}

/** True once the process's file is on Commons (before that its File: page may not exist). */
export function iframeFileOnCommons(status: TaskProcessStatusEnum) {
  return [TaskProcessStatusEnum.Uploaded, TaskProcessStatusEnum.Overwritten, TaskProcessStatusEnum.Skipped, TaskProcessStatusEnum.DescriptionUpdated].includes(status);
}

/** A task's {{OWID}} block as shown: provisional until the upload is done. */
export interface IframeWikiTextState {
  wikiText: string
  final: boolean
  // Provisional blocks shown so far (the user may have copied any of them)
  provisionalSeen: string[]
  // The final block differs from a provisional one that was shown
  changed: boolean
}

export const EMPTY_IFRAME_WIKITEXT: IframeWikiTextState = { wikiText: "", final: false, provisionalSeen: [], changed: false };

/** The state after a fetch returned `wikiText` (final or provisional). */
export function trackIframeWikiText(prev: IframeWikiTextState, wikiText: string, final: boolean): IframeWikiTextState {
  const provisionalSeen = !final && wikiText && !prev.provisionalSeen.includes(wikiText)
    ? [...prev.provisionalSeen, wikiText]
    : prev.provisionalSeen;
  const changed = final && !!wikiText && provisionalSeen.some(seen => seen !== wikiText);
  return { wikiText, final, provisionalSeen, changed };
}
