import { DescriptionOverwriteBehaviour, MapImporterFormItem } from "./types";

export const SESSION_ID_KEY = "sessionId";
export const USERNAME_KEY = "username";
export const COMMONS_TEMPLATE_PREFIX = "Template:OWID"
export const OWID_CHART_URL_PREFIX = "https://ourworldindata.org/grapher"


export const URL_PLACEHOLDER = `https://ourworldindata.org/grapher/<NAME OF GRAPH>`;
export const INITIAL_CATEGORIES_MAP = [
  "$YEAR maps of {{subst:#ifeq:$REGION|World|the world|$REGION}}",
  "SVG maps by Our World in Data",
  "Uploaded by OWID importer tool"
]

export const CHART_INFO_MAP = `You can use $NAME (filename without extension), $YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders. This only works for graphs that are maps with data over multiple years.`;

export const INITIAL_CATEGORIES_CHART = ["Uploaded by OWID importer tool"]
export const CHART_INFO_CHART = `You can use $NAME (filename without extension), $START_YEAR, $END_YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders`;


const INITIAL_TEMPLATE_NAME = `$CHART_NAME`
const INITIAL_DESCRIPTION_MAP = `=={{int:filedesc}}==
{{Information
|description={{en|1=$TITLE, $REGION}}
|author = Our World In Data
|date= $YEAR
|source = $URL
|permission = "License: All of Our World in Data is completely open access and all work is licensed under the Creative Commons BY license. You have the permission to use, distribute, and reproduce in any medium, provided the source and authors are credited."
|other versions =
}}
{{Map showing old data|year=$YEAR}}
=={{int:license-header}}==
{{cc-by-4.0}}
`;

const INITIAL_FILENAME_MAP = `$NAME, $REGION, $YEAR.svg`;

const INITIAL_DESCRIPTION_CHART = `=={{int:filedesc}}==
{{Information
|description={{en|1=$TITLE, $REGION}}
|author = Our World In Data
|date= $END_YEAR
|source = $URL
|permission = "License: All of Our World in Data is completely open access and all work is licensed under the Creative Commons BY license. You have the permission to use, distribute, and reproduce in any medium, provided the source and authors are credited."
|other versions =
}}
=={{int:license-header}}==
{{cc-by-4.0}}
`;
// [[Category:Uploaded by OWID importer tool]]
const INITIAL_FILENAME_CHART = `$NAME, $START_YEAR to $END_YEAR, $REGION.svg`;

const initialImport: MapImporterFormItem = {
  id: Date.now().toString(),
  url: "",
  fileName: INITIAL_FILENAME_MAP,
  description: INITIAL_DESCRIPTION_MAP,
  categories: INITIAL_CATEGORIES_MAP,
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour.ALL,
  importCountries: true,
  generateTemplateCommons: true,
  selectedChartParameters: [],
  templateNameFormat: INITIAL_TEMPLATE_NAME,

  // Country
  countryFileName: INITIAL_FILENAME_CHART,
  countryDescription: INITIAL_DESCRIPTION_CHART,
  countryCategories: INITIAL_CATEGORIES_CHART,
  countryDescriptionOverwriteBehaviour: DescriptionOverwriteBehaviour.ALL,
  linkVerified: false,
  templateExists: false,
  canImport: false
}

export function generateBlankImport(): MapImporterFormItem {
  return {
    ...initialImport,
    id: Date.now().toString()
  }
}
