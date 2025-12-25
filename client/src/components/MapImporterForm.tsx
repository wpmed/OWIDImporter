import { Box, Button, Checkbox, InputAdornment, Radio, Stack, TextareaAutosize, TextField, Typography } from "@mui/material";
import { DescriptionOverwriteBehaviour, SelectedParameter } from "../types"
import { CategoriesSearchInput } from "./CategoriesSearchInput";
import { useDebounce } from "use-debounce";
import { useCallback, useEffect, useState } from "react";
import { getChartParameters } from "../request/request";
import { Delete } from "@mui/icons-material";

export const DESCRIPTION_OVERWRITE_OPTIONS = [
  {
    value: DescriptionOverwriteBehaviour.ALL,
    title: "Overwrite full description",
    description: "Overwrite the full description of the file (if already exists) with the new description.",
  },
  {
    value: DescriptionOverwriteBehaviour.ALL_EXCEPT_CATEGORIES,
    title: "Overwrite description except the categories",
    description: "Old categories are retained, any new categories in the new description are discarded/skipped. If the file doesn't already exist, categories in the new description are added.",
  },
  {
    value: DescriptionOverwriteBehaviour.ONLY_FILE,
    title: "Only upload file",
    description: "Don't update the description, only upload the file.",
  }
]

export interface MapImporterFormItem {
  id:string 
  url: string
  fileName: string
  description: string
  categories: string[]
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour

  countryFileName: string
  countryDescription: string
  countryCategories: string[]
  countryDescriptionOverwriteBehaviour: DescriptionOverwriteBehaviour
  importCountries: boolean
  generateTemplateCommons: boolean
  selectedChartParameters: SelectedParameter[]

  templateNameFormat: string
}


export interface MapImporterFormProps {
  disabled: boolean
  value: MapImporterFormItem
  onChange: (val: MapImporterFormItem) => void
  onParamtersLoadingChange: (val: boolean) => void
  onDelete: () => void,
  canRemove: boolean
}

const url_placeholder = `https://ourworldindata.org/grapher/<NAME OF GRAPH>`;
const initial_categories_map = [
  "$YEAR maps of {{subst:#ifeq:$REGION|World|the world|$REGION}}",
  "SVG maps by Our World in Data",
  "Uploaded by OWID importer tool"
]

const chart_info_map = `You can use $NAME (filename without extension), $YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders. This only works for graphs that are maps with data over multiple years.`;
const initial_categories_chart = ["Uploaded by OWID importer tool"]
const chart_info_chart = `You can use $NAME (filename without extension), $START_YEAR, $END_YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders`;

// { url, fileName, description, categories, descriptionOverwriteBehaviour, countryFileName, countryDescription, countryCategories, countryDescriptionOverwriteBehaviour, importCountries, generateTemplateCommons, selectedChartParameters, templateName }
export function MapImporterForm({ value, onChange, onDelete, disabled, onParamtersLoadingChange, canRemove }: MapImporterFormProps) {
  const [debouncedUrl] = useDebounce(value.url, 1000);
  const [lastCheckedUrl, setLastCheckeUrl] = useState(value.url);

  const handleChange = useCallback(<K extends keyof MapImporterFormItem>(
    key: K,
    val: MapImporterFormItem[K]
  ) => {
    onChange({ ...value, [key]: val });
  }, [value, onChange]);

  useEffect(() => {
    if (!disabled && debouncedUrl && debouncedUrl.includes("?") && debouncedUrl != lastCheckedUrl) {
      setLastCheckeUrl(debouncedUrl);

      onParamtersLoadingChange(true);
      getChartParameters(debouncedUrl)
        .then(res => {
          if (res.params && res.params.length > 0) {
            // setChartParameters(res.params);
            const paramsKeys = res.params.map(param => param.slug);
            const parts = debouncedUrl.split("?").pop()?.split("&")
            const selectedParams: SelectedParameter[] = []
            let newInitialFilenameMap = "$NAME";
            let newInitialFilenameChart = "$NAME";
            let newTemplateName = "$CHART_NAME"
            parts?.forEach(part => {
              const [key, val] = part.split("=");
              if (key && val && paramsKeys.includes(key)) {
                const param = res.params.find(p => p.slug == key)
                const choice = param?.choices.find(c => c.slug == val)
                if (param && choice) {
                  selectedParams.push({ key: param.slug, keyName: param.name, value: val, valueName: choice.name })
                } else {
                  selectedParams.push({ key, keyName: key, value: val, valueName: val })
                }
                newInitialFilenameMap += `, $${key.toUpperCase()}`;
                newInitialFilenameChart += `, $${key.toUpperCase()}`;
                newTemplateName += `, $${key.toUpperCase()}`;
              }
            })

            newInitialFilenameMap += ", $REGION, $YEAR.svg";
            newInitialFilenameChart += ", $START_YEAR to $END_YEAR, $REGION.svg";

            onChange({
              ...value,
              fileName: newInitialFilenameMap,
              countryFileName: newInitialFilenameChart,
              selectedChartParameters: selectedParams,
              templateNameFormat: newTemplateName,
            });
          }
        })
        .catch(err => {
          console.log("Error getting chart parameters", err)
        })
        .finally(() => {
          onParamtersLoadingChange(false);
        })
    }
  }, [debouncedUrl, disabled, lastCheckedUrl, value, onChange, onParamtersLoadingChange])

  return (
    <Stack>
      <Stack spacing={2}>
        <Stack flexDirection={"row"}>
          <Stack>
            <Typography variant="h4">
              <span>Map</span>
            </Typography>
            <Typography>
              <span dangerouslySetInnerHTML={{ __html: chart_info_map }} />
            </Typography>
          </Stack>
          {canRemove ? (
            <Stack>
              <Button onClick={onDelete} startIcon={<Delete />} color="error" >Remove</Button>
            </Stack>
          ) : null}
        </Stack>

        <Stack spacing={1}>
          <Typography>File URL</Typography>
          <TextField
            fullWidth
            size="small"
            value={value.url}
            onChange={e => handleChange("url", e.target.value)}
            placeholder={url_placeholder}
            disabled={disabled}
          />
        </Stack>

        {value.selectedChartParameters.length > 0 && (
          <Stack spacing={1}>
            <Typography variant="h6" >Selected parameters</Typography>
            {value.selectedChartParameters.map(param => (
              <Typography>{param.keyName}: {param.valueName} - You can use <strong>${param.key.toUpperCase()}</strong> in the file name as a placeholder</Typography>
            ))}
          </Stack>
        )}

        <Stack spacing={3}>
          <Stack spacing={1}>
            <Typography>File name</Typography>
            <TextField
              size="small"
              value={value.fileName}
              onChange={e => handleChange("fileName", e.target.value)}
              fullWidth
              disabled={disabled}
            />
          </Stack>
          <Stack spacing={1}>
            <Typography>Description</Typography>
            <TextareaAutosize
              value={value.description}
              onChange={e => handleChange('description', e.target.value)}
              style={{ width: "100%", backgroundColor: "white", color: "black" }}
              minRows={5}
              disabled={disabled}
            />
          </Stack>
          <Stack spacing={1}>
            <Stack direction="row" justifyContent="space-between">
              <Typography>Categories</Typography>
              <Button onClick={() => handleChange("categories", initial_categories_map)} size="small">Reset</Button>
            </Stack>
            <CategoriesSearchInput value={value.categories} onChange={(newCategories) => handleChange("categories", newCategories)} disabled={disabled} />
          </Stack>
        </Stack>

        <Stack spacing={1}>
          <Typography>
            If a file with the same name exists:
          </Typography>
          {DESCRIPTION_OVERWRITE_OPTIONS.map(option => (
            <Stack spacing={1}>
              <Stack direction={"row"} alignItems={"flex-start"}>
                <Radio disabled={disabled} checked={value.descriptionOverwriteBehaviour == option.value} onClick={() => handleChange("descriptionOverwriteBehaviour", option.value)} />
                <Box>
                  <Typography>
                    {option.title}
                  </Typography>
                  <Typography variant="subtitle2">{option.description}</Typography>
                </Box>
              </Stack>
            </Stack>
          ))}
        </Stack>

        <Stack spacing={1}>
          <Stack direction="row" alignItems={"center"} >
            <Checkbox checked={value.generateTemplateCommons} onClick={() => handleChange("generateTemplateCommons", !value.generateTemplateCommons)} disabled={disabled} />
            <Typography>Automatically Create Template Page On Commons</Typography>
          </Stack>
          {value.generateTemplateCommons && (
            <Stack spacing={1}>
              <Typography>Template name</Typography>
              <TextField
                size="small"
                value={value.templateNameFormat}
                onChange={e => handleChange("templateNameFormat", e.target.value)}
                fullWidth
                disabled={disabled}
                slotProps={{
                  input: {
                    startAdornment: <InputAdornment position="start">Template:OWID/</InputAdornment>,
                  },
                }}
              />
            </Stack>
          )}
        </Stack>


        <Stack>
          <Stack direction="row" alignItems={"center"} >
            <Checkbox checked={value.importCountries} disabled={disabled} onClick={() => handleChange("importCountries", !value.importCountries)} />
            <Typography>Import Countries</Typography>
          </Stack>
        </Stack>
        {
          value.importCountries && (
            <>
              <Stack spacing={2}>
                <Typography variant="h4">
                  <span>Country Chart</span>
                </Typography>
                <Typography>
                  <span dangerouslySetInnerHTML={{ __html: chart_info_chart }} />
                </Typography>
              </Stack>
              <Stack spacing={1}>
                <Typography>File name</Typography>
                <TextField
                  size="small"
                  value={value.countryFileName}
                  onChange={e => handleChange("countryFileName", e.target.value)}
                  fullWidth
                  disabled={disabled}
                />
              </Stack>
              <Stack spacing={1}>
                <Typography>Description</Typography>
                <TextareaAutosize
                  value={value.countryDescription}
                  onChange={e => handleChange("countryDescription", e.target.value)}
                  style={{ width: "100%", backgroundColor: "white", color: "black" }}
                  minRows={5}
                  disabled={disabled}
                />
              </Stack>
              <Stack spacing={1}>
                <Stack direction="row" justifyContent="space-between">
                  <Typography>Categories</Typography>
                  <Button onClick={() => handleChange("countryCategories", initial_categories_chart)} disabled={disabled} size="small" >Reset</Button>
                </Stack>
                <CategoriesSearchInput value={value.countryCategories} onChange={(newCategories) => handleChange("countryCategories", newCategories)} disabled={disabled} />
              </Stack>
              <Stack spacing={1}>
                <Typography>
                  If a file with the same name exists:
                </Typography>
                {DESCRIPTION_OVERWRITE_OPTIONS.map(option => (
                  <Stack spacing={1}>
                    <Stack direction={"row"} alignItems={"flex-start"}>
                      <Radio disabled={disabled} checked={value.countryDescriptionOverwriteBehaviour == option.value} onClick={() => handleChange("countryDescriptionOverwriteBehaviour", option.value)} />
                      < Box >
                        <Typography>
                          {option.title}
                        </Typography>
                        <Typography variant="subtitle2">{option.description}</Typography>
                      </Box>
                    </Stack>
                  </Stack>
                ))}
              </Stack>
            </>
          )
        }
      </Stack>
    </Stack >
  )
}
