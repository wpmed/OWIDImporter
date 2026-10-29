import { OWID_CHART_URL_PREFIX } from "./constants";
import { applyChartSourceToDescription } from "./utils";
import { IframeImporterFormItem, IframeInfo, TaskProcess, TaskProcessStatusEnum } from "./types";

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
  if (!IFRAME_CHART_PATH_REGEX.test(path) || FORBIDDEN_CHART_PATH_REGEX.test(path)) {
    return null;
  }
  return path;
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

/** The Commons file name an iframe task ended up with (the task's filename is only the `$NAME.svg` template). */
export function iframeFinalFileName(processes: Pick<TaskProcess, "filename" | "status">[]): string | undefined {
  const done = processes.find(p => p.filename && p.status !== TaskProcessStatusEnum.Failed);
  return done?.filename.replace(/^File:/, "");
}
