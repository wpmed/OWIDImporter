import { Alert, Box, Button, Paper, Stack, TextField, Typography } from "@mui/material";
import { Delete } from "@mui/icons-material";
import { useCallback, useEffect, useRef, useState } from "react";
import { useDebounce } from "use-debounce";
import { DescriptionOverwriteBehaviour, IframeImporterFormItem } from "../types";
import { CategoriesSearchInput } from "./CategoriesSearchInput";
import { FieldLoading } from "./FieldLoader";
import { OverwriteBehaviourField } from "./OverwriteBehaviourField";
import { PlaceholderText } from "./ui/PlaceholderText";
import { IFRAME_DESCRIPTION_OVERWRITE_OPTIONS, IFRAME_INFO, IFRAME_URL_PLACEHOLDER, INITIAL_CATEGORIES_IFRAME } from "../constants";
import { getIframeInfo } from "../request/request";
import { applyIframeLookupFailure, applyIframeLookupInfo, editIframeUrl, IframeItemUpdate, normalizeIframeChartPath } from "../iframe";
import { monoStack, serifStack } from "../theme";

export interface IframeImporterFormProps {
  disabled: boolean
  value: IframeImporterFormItem
  // Receives an update derived from the latest item (never a snapshot), so async results cannot clobber newer edits
  onChange: (update: IframeItemUpdate) => void
  onInfoLoadingChange: (val: boolean) => void
  onDelete: () => void
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

export function IframeImporterForm({ value, onChange, onDelete, disabled, onInfoLoadingChange, canRemove }: IframeImporterFormProps) {
  const [debouncedUrl] = useDebounce(value.url, 1000);
  const [lastCheckedUrl, setLastCheckedUrl] = useState(value.linkVerified ? value.url : "");
  const [infoLoading, setInfoLoading] = useState(false);
  // Source text of the previous chart when the user edited the description before changing the URL;
  // the next successful lookup swaps it for the new chart's source.
  const previousSourceRef = useRef<string | null>(null);
  // Only the newest lookup controls the loading indicator.
  const lookupIdRef = useRef(0);
  // Latest URL, so a lookup that finishes after the URL changed can tell its result was dropped.
  const urlRef = useRef(value.url);
  urlRef.current = value.url;

  const handleChange = useCallback(<K extends keyof IframeImporterFormItem>(
    key: K,
    val: IframeImporterFormItem[K]
  ) => {
    onChange(prev => ({ ...prev, [key]: val }));
  }, [onChange]);

  const onUrlChange = (url: string) => {
    onChange(prev => {
      const { item, previousSource } = editIframeUrl(prev, url);
      // Idempotent, so safe if React runs the updater twice
      if (previousSource !== undefined) {
        previousSourceRef.current = previousSource;
      }
      return item;
    });
  }


  // The pending swap was consumed by the lookup that just succeeded.
  useEffect(() => {
    if (value.canImport) {
      previousSourceRef.current = null;
    }
  }, [value.canImport]);

  useEffect(() => {
    if (disabled || !debouncedUrl.trim() || debouncedUrl !== value.url || debouncedUrl === lastCheckedUrl || value.linkVerified) {
      return;
    }
    setLastCheckedUrl(debouncedUrl);

    const path = normalizeIframeChartPath(debouncedUrl);
    if (!path) {
      onChange(prev => applyIframeLookupFailure(prev, debouncedUrl));
      return;
    }

    // Every result below is applied to the latest item and dropped when its URL is no longer debouncedUrl.
    const lookupId = ++lookupIdRef.current;
    onInfoLoadingChange(true);
    setInfoLoading(true);
    getIframeInfo(path)
      .then(res => {
        if (res.error || !res.info) {
          onChange(prev => applyIframeLookupFailure(prev, debouncedUrl));
          return;
        }
        const info = res.info;
        const previousSource = previousSourceRef.current ?? undefined;
        onChange(prev => applyIframeLookupInfo(prev, debouncedUrl, info, previousSource));
      })
      .catch(err => {
        console.log("Error getting iframe info", err);
        onChange(prev => applyIframeLookupFailure(prev, debouncedUrl));
      })
      .finally(() => {
        // The result was dropped as stale: allow this URL to be looked up again if the user returns to it.
        if (urlRef.current !== debouncedUrl) {
          setLastCheckedUrl(checked => checked === debouncedUrl ? "" : checked);
        }
        if (lookupId === lookupIdRef.current) {
          onInfoLoadingChange(false);
          setInfoLoading(false);
        }
      })
  }, [disabled, debouncedUrl, lastCheckedUrl, value.url, value.linkVerified, onChange, onInfoLoadingChange])

  const info = value.info;

  return (
    <Stack spacing={3}>
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
        <Stack spacing={0.5}>
          <Typography variant="h5" sx={{ fontFamily: serifStack }}>Iframe</Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            <PlaceholderText text={IFRAME_INFO} />
          </Typography>
        </Stack>
        {canRemove && (
          <Button onClick={onDelete} startIcon={<Delete />} color="error" size="small" disabled={disabled}>
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
          onChange={e => onUrlChange(e.target.value)}
          placeholder={IFRAME_URL_PLACEHOLDER}
          helperText="A full ourworldindata.org URL or a path such as grapher/life-expectancy or explorers/minerals?tab=map"
          disabled={disabled}
          sx={monoInputSx}
        />
        {infoLoading && (
          <FieldLoading />
        )}
      </Box>

      {info && (info.chartName || info.title || info.source) && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Stack spacing={1}>
            <Typography variant="overline" sx={{ color: 'text.secondary', lineHeight: 1.5 }}>
              Chart details
            </Typography>
            {info.chartName && (
              <ChartDetailRow label="Name" value={info.chartName} />
            )}
            {info.title && (
              <ChartDetailRow label="Title" value={info.title} />
            )}
            {info.source && (
              <ChartDetailRow label="Data source" value={info.source} />
            )}
          </Stack>
        </Paper>
      )}

      {!infoLoading && value.linkVerified && !value.canImport && (
        <Alert severity="error" variant="outlined">
          This chart cannot be imported.
        </Alert>
      )}

      <Stack spacing={1}>
        <TextField
          size="small"
          label="File name"
          value={value.fileName}
          onChange={e => handleChange("fileName", e.target.value)}
          fullWidth
          disabled={disabled}
          sx={monoInputSx}
        />
      </Stack>

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
          <Button onClick={() => handleChange("categories", INITIAL_CATEGORIES_IFRAME)} disabled={disabled} size="small">Reset</Button>
        </Stack>
        <CategoriesSearchInput value={value.categories} onChange={(newCategories) => handleChange("categories", newCategories)} disabled={disabled} />
      </Stack>

      <OverwriteBehaviourField
        options={IFRAME_DESCRIPTION_OVERWRITE_OPTIONS}
        value={value.descriptionOverwriteBehaviour}
        onChange={(val) => handleChange("descriptionOverwriteBehaviour", val as DescriptionOverwriteBehaviour)}
        disabled={disabled}
      />
    </Stack>
  )
}
