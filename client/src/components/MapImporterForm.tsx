import { Alert, Box, Button, Checkbox, FormControlLabel, InputAdornment, Link, Paper, Stack, TextField, Typography } from "@mui/material";
import { DescriptionOverwriteBehaviour, MapImporterFormItem, SelectedParameter } from "../types"
import { CategoriesSearchInput } from "./CategoriesSearchInput";
import { useDebounce } from "use-debounce";
import { useCallback, useEffect, useState } from "react";
import { ChartInfo, getChartParameters } from "../request/request";
import { Delete } from "@mui/icons-material";
import { CHART_INFO_CHART, CHART_INFO_MAP, COMMONS_TEMPLATE_PREFIX, COUNTRY_DESCRIPTION_OVERWRITE_OPTIONS, DESCRIPTION_OVERWRITE_OPTIONS, INITIAL_CATEGORIES_CHART, INITIAL_CATEGORIES_MAP, INITIAL_CATEGORIES_MAP_SINGLE_IMAGE, INITIAL_DESCRIPTION_MAP, INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE, INITIAL_FILENAME_MAP, INITIAL_FILENAME_MAP_SINGLE_IMAGE, OWID_CHART_URL_PREFIX, URL_PLACEHOLDER } from "../constants";
import { searchPageExists } from "../request/commons";
import { FieldLoading } from "./FieldLoader";
import { PlaceholderText } from "./ui/PlaceholderText";
import { OverwriteBehaviourField } from "./OverwriteBehaviourField";
import { monoStack, serifStack } from "../theme";

export interface MapImporterFormProps {
  disabled: boolean
  value: MapImporterFormItem
  onChange: (val: MapImporterFormItem) => void
  onParamtersLoadingChange: (val: boolean) => void
  onDelete: () => void,
  canRemove: boolean
}

const monoInputSx = {
  '& .MuiInputBase-input, & .MuiInputBase-inputMultiline': {
    fontFamily: monoStack,
    fontSize: 14,
  },
} as const;

function ChartDetailRow({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" spacing={1}>
      <Typography variant="body2" sx={{ color: 'text.secondary', width: 88, flexShrink: 0 }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ fontWeight: 500 }}>
        {value}
      </Typography>
    </Stack>
  )
}

export function MapImporterForm({ value, onChange, onDelete, disabled, onParamtersLoadingChange, canRemove }: MapImporterFormProps) {
  const [debouncedUrl] = useDebounce(value.url, 1000);
  const [debouncedTemplateName] = useDebounce(value.templateNameFormat, 1000);
  const [lastCheckedUrl, setLastCheckeUrl] = useState(value.url);
  const [chartInfo, setChartInfo] = useState<ChartInfo | null>(null);
  const [lastCheckedTemplateName, setLastCheckedTemplateName] = useState("");
  const [templateExistsLoading, setTemplateExistsLoading] = useState(false);
  const [templateExists, setTemplateExists] = useState(false);
  const [parametersLoading, setParametersLoading] = useState(false);

  const handleChange = useCallback(<K extends keyof MapImporterFormItem>(
    key: K,
    val: MapImporterFormItem[K]
  ) => {
    onChange({ ...value, [key]: val });
  }, [value, onChange]);

  useEffect(() => {
    if (!disabled && debouncedUrl && debouncedUrl != lastCheckedUrl && debouncedUrl.startsWith(OWID_CHART_URL_PREFIX) && !value.linkVerified) {
      setLastCheckeUrl(debouncedUrl);
      setChartInfo(null);

      onParamtersLoadingChange(true);
      setParametersLoading(true);
      getChartParameters(debouncedUrl)
        .then(res => {
          if (res.error) {
            console.log("Chart info error: ", res);
            onChange({ ...value, canImport: false, linkVerified: true });
          } else if (res.params && res.params.length > 0) {
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
            newInitialFilenameChart += ", $REGION.svg";

            onChange({
              ...value,
              singleImage: res.info.singleImage,
              fileName: newInitialFilenameMap,
              countryFileName: newInitialFilenameChart,
              selectedChartParameters: selectedParams,
              templateNameFormat: newTemplateName,
              linkVerified: true,
              canImport: true
            });
          } else {
            let fileName = INITIAL_FILENAME_MAP;
            let description = INITIAL_DESCRIPTION_MAP;
            let categories = INITIAL_CATEGORIES_MAP;

            if (res.info.singleImage) {
              fileName = INITIAL_FILENAME_MAP_SINGLE_IMAGE;
              description = INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE;
              categories = INITIAL_CATEGORIES_MAP_SINGLE_IMAGE;
            }

            onChange({
              ...value,
              fileName,
              description,
              categories,
              singleImage: res.info.singleImage,
              linkVerified: true,
              canImport: true
            });
          }

          if (res.info) {
            setChartInfo(res.info);
          }
        })
        .catch(err => {
          console.log("Error getting chart parameters", err)
        })
        .finally(() => {
          onParamtersLoadingChange(false);
          setParametersLoading(false);
        })
    }
  }, [disabled, debouncedUrl, lastCheckedUrl, value, onChange, onParamtersLoadingChange, setChartInfo, setParametersLoading])

  useEffect(() => {
    if (chartInfo) {
      let templateName = `${COMMONS_TEMPLATE_PREFIX}/${debouncedTemplateName}`;
      templateName = templateName.replace("$CHART_NAME", chartInfo?.chartName);
      if (value.selectedChartParameters.length > 0) {
        value.selectedChartParameters.forEach((param) => {
          templateName = templateName.replace(`$${param.key.toUpperCase()}`, param.valueName);
        })
      }
      if (templateName != lastCheckedTemplateName) {
        setLastCheckedTemplateName(templateName);
        setTemplateExistsLoading(true);
        searchPageExists(templateName)
          .then((exists) => {
            setTemplateExists(exists);
          })
          .catch(err => {
            console.log("Error checking template exists: ", err);
          })
          .finally(() => {
            setTemplateExistsLoading(false);
          })
      }
    }

  }, [chartInfo, debouncedTemplateName, value.selectedChartParameters, lastCheckedTemplateName, setLastCheckedTemplateName, setTemplateExistsLoading, setTemplateExists]);

  return (
    <Stack spacing={3}>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
        <Stack spacing={0.5}>
          <Typography variant="h5" sx={{ fontFamily: serifStack }}>Map</Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            <PlaceholderText text={CHART_INFO_MAP} />
          </Typography>
        </Stack>
        {canRemove && (
          <Button onClick={onDelete} startIcon={<Delete />} color="error" size="small">
            Remove
          </Button>
        )}
      </Stack>

      <Box sx={{ position: "relative" }}>
        <TextField
          fullWidth
          size="small"
          label="Chart URL"
          value={value.url}
          onChange={e => handleChange("url", e.target.value)}
          placeholder={URL_PLACEHOLDER}
          disabled={disabled}
          sx={monoInputSx}
        />
        {parametersLoading && (
          <FieldLoading />
        )}
      </Box>

      {chartInfo?.singleImage && (
        <Alert severity="warning" variant="outlined">
          This chart is composed of a single image and will be uploaded as such.
        </Alert>
      )}

      {chartInfo && (chartInfo.chartName || chartInfo.title || (chartInfo.startYear && chartInfo.endYear)) && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Stack spacing={1}>
            <Typography variant="overline" sx={{ color: 'text.secondary', lineHeight: 1.5 }}>
              Chart details
            </Typography>
            {chartInfo.chartName && (
              <ChartDetailRow label="Name" value={chartInfo.chartName} />
            )}
            {chartInfo.title && (
              <ChartDetailRow label="Title" value={chartInfo.title} />
            )}
            {chartInfo.startYear && chartInfo.endYear && (
              <ChartDetailRow label="Years" value={`${chartInfo.startYear} – ${chartInfo.endYear}`} />
            )}
          </Stack>
        </Paper>
      )}

      {!parametersLoading && value.linkVerified && !value.canImport && (
        <Alert severity="error" variant="outlined">
          This chart cannot be imported.
        </Alert>
      )}

      {value.selectedChartParameters.length > 0 && (
        <Stack spacing={1}>
          <Typography sx={{ fontWeight: 600 }}>Selected parameters</Typography>
          {value.selectedChartParameters.map(param => (
            <Typography key={param.key} variant="body2" sx={{ color: 'text.secondary' }}>
              {param.keyName}: {param.valueName} — <PlaceholderText text={`you can use $${param.key.toUpperCase()} in the file name as a placeholder`} />
            </Typography>
          ))}
        </Stack>
      )}

      <TextField
        size="small"
        label="File name"
        value={value.fileName}
        onChange={e => handleChange("fileName", e.target.value)}
        fullWidth
        disabled={disabled}
        sx={monoInputSx}
      />

      <TextField
        label="Description"
        value={value.description}
        onChange={e => handleChange('description', e.target.value)}
        multiline
        minRows={5}
        fullWidth
        disabled={disabled}
        sx={monoInputSx}
      />

      <Stack spacing={1}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="body2" sx={{ fontWeight: 600 }}>Categories</Typography>
          <Button onClick={() => handleChange("categories", INITIAL_CATEGORIES_MAP)} size="small">Reset</Button>
        </Stack>
        <CategoriesSearchInput value={value.categories} onChange={(newCategories) => handleChange("categories", newCategories)} disabled={disabled} />
      </Stack>

      <OverwriteBehaviourField
        options={DESCRIPTION_OVERWRITE_OPTIONS}
        value={value.descriptionOverwriteBehaviour}
        onChange={(val) => handleChange("descriptionOverwriteBehaviour", val as DescriptionOverwriteBehaviour)}
        disabled={disabled}
      />

      {!value.singleImage && (
        <Stack spacing={1}>
          <FormControlLabel
            control={(
              <Checkbox
                checked={value.generateTemplateCommons}
                onClick={() => handleChange("generateTemplateCommons", !value.generateTemplateCommons)}
                disabled={disabled}
              />
            )}
            label="Automatically create template page on Commons"
          />
          {value.generateTemplateCommons && (
            <Stack spacing={1.5}>
              <Box sx={{ position: "relative" }}>
                <TextField
                  size="small"
                  label="Template name"
                  value={value.templateNameFormat}
                  onChange={e => handleChange("templateNameFormat", e.target.value)}
                  fullWidth
                  disabled={disabled}
                  sx={monoInputSx}
                  slotProps={{
                    input: {
                      startAdornment: <InputAdornment position="start">Template:OWID/</InputAdornment>,
                    },
                  }}
                />
                {templateExistsLoading && (
                  <FieldLoading />
                )}
              </Box>
              {templateExists && lastCheckedTemplateName && (
                <Alert severity="warning" variant="outlined">
                  A template with this name{' '}
                  <Link
                    href={`${import.meta.env.VITE_MW_BASE_URL}/${lastCheckedTemplateName}`}
                    target="_blank"
                    color="inherit"
                  >
                    already exists
                  </Link>.
                </Alert>
              )}
            </Stack>
          )}
        </Stack>
      )}

      {!value.singleImage && (
        <>
          <FormControlLabel
            control={(
              <Checkbox
                checked={value.importCountries}
                disabled={disabled}
                onClick={() => handleChange("importCountries", !value.importCountries)}
              />
            )}
            label="Import countries"
          />
          {value.importCountries && (
            <Stack
              spacing={3}
              sx={{ borderLeft: '2px solid', borderColor: 'secondary.main', pl: { xs: 2, md: 3 } }}
            >
              <Stack spacing={0.5}>
                <Typography variant="h5" sx={{ fontFamily: serifStack }}>Country chart</Typography>
                {chartInfo && !chartInfo.hasCountries && (
                  <Alert severity="warning" variant="outlined" sx={{ my: 1 }}>
                    This chart doesn't support countries line charts. We'll try with popover charts.
                  </Alert>
                )}
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                  <PlaceholderText text={CHART_INFO_CHART} />
                </Typography>
              </Stack>
              <TextField
                size="small"
                label="File name"
                value={value.countryFileName}
                onChange={e => handleChange("countryFileName", e.target.value)}
                fullWidth
                disabled={disabled}
                sx={monoInputSx}
              />
              <TextField
                label="Description"
                value={value.countryDescription}
                onChange={e => handleChange("countryDescription", e.target.value)}
                multiline
                minRows={5}
                fullWidth
                disabled={disabled}
                sx={monoInputSx}
              />
              <Stack spacing={1}>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>Categories</Typography>
                  <Button onClick={() => handleChange("countryCategories", INITIAL_CATEGORIES_CHART)} disabled={disabled} size="small">Reset</Button>
                </Stack>
                <CategoriesSearchInput value={value.countryCategories} onChange={(newCategories) => handleChange("countryCategories", newCategories)} disabled={disabled} />
              </Stack>
              <OverwriteBehaviourField
                options={COUNTRY_DESCRIPTION_OVERWRITE_OPTIONS}
                value={value.countryDescriptionOverwriteBehaviour}
                onChange={(val) => handleChange("countryDescriptionOverwriteBehaviour", val as DescriptionOverwriteBehaviour)}
                disabled={disabled}
              />
            </Stack>
          )}
        </>
      )}
    </Stack>
  )
}
