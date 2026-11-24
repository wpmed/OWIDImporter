import { INITIAL_DESCRIPTION_MAP, INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE } from "./constants";
import { applySourceToEditedDescription, chartSourceCredit } from "./iframe";
import { MapImporterFormItem } from "./types";

/**
 * The item after the user edits the Chart URL. A different URL invalidates the previous lookup, so the
 * verification and the chart-derived values it set are cleared and the next lookup runs for the new URL.
 * The same URL returns `prev` itself. File name and categories are kept (the user may have edited them; a
 * successful lookup rewrites them anyway).
 *
 * The previous chart's source credit that the lookup put in place of `$SOURCE` is taken back out of a
 * description the user has not touched. A description they edited stays as typed, and the previous credit
 * is remembered so the next lookup can swap it for the new chart's source.
 */
export function editMapUrl(prev: MapImporterFormItem, url: string): MapImporterFormItem {
  if (url === prev.url) {
    return prev;
  }
  const { sourceApplied } = prev;
  const descriptionUntouched = !!sourceApplied && prev.description === sourceApplied.description.applied;
  const countryUntouched = !!sourceApplied && prev.countryDescription === sourceApplied.countryDescription.applied;
  return {
    ...prev,
    url,
    description: descriptionUntouched ? sourceApplied.description.template : prev.description,
    countryDescription: countryUntouched ? sourceApplied.countryDescription.template : prev.countryDescription,
    sourceApplied: undefined,
    // Kept across several edits before a lookup answers
    previousSourceCredit: sourceApplied && !(descriptionUntouched && countryUntouched) ? sourceApplied.credit : prev.previousSourceCredit,
    linkVerified: false,
    canImport: false,
    singleImage: false,
    selectedChartParameters: [],
  };
}

/**
 * Puts the chart's source into the item's descriptions (`item.description` / `countryDescription` are the
 * texts to fill) and records what was substituted so a later URL edit can undo it.
 */
export function applyMapSource(item: MapImporterFormItem, source?: string): MapImporterFormItem {
  const description = applySourceToEditedDescription(item.description, source, item.previousSourceCredit);
  const countryDescription = applySourceToEditedDescription(item.countryDescription, source, item.previousSourceCredit);
  return {
    ...item,
    description: description.description,
    countryDescription: countryDescription.description,
    sourceApplied: {
      credit: chartSourceCredit(source),
      description: { template: description.template, applied: description.description },
      countryDescription: { template: countryDescription.template, applied: countryDescription.description },
    },
    previousSourceCredit: undefined,
  };
}

/**
 * The map description a lookup without chart parameters starts from: the default template for the chart
 * (`template`) when the description is still a default or what the last lookup filled in, and the user's
 * text when they typed their own.
 */
export function mapLookupDescription(item: MapImporterFormItem, template: string): string {
  const untouched = item.description === INITIAL_DESCRIPTION_MAP
    || item.description === INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE
    || (!!item.sourceApplied && item.description === item.sourceApplied.description.applied);
  return untouched ? template : item.description;
}
