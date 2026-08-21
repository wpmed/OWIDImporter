import { Alert, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Link, Stack, TextField, Typography } from "@mui/material"
import { useCallback, useMemo, useState } from "react";
import { COMMONS_TEMPLATE_PREFIX, DESCRIPTION_OVERWRITE_OPTIONS, generateBlankImport, INITIAL_CATEGORIES_MAP_SINGLE_IMAGE, INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE, INITIAL_FILENAME_MAP_SINGLE_IMAGE, OWID_CHART_URL_PREFIX } from "../constants";
import { getChartParameters } from "../request/request";
import pLimit from 'p-limit';
import { CheckCircle, Close as CloseIcon } from "@mui/icons-material";
import { MapImporterFormItem, SelectedParameter } from "../types";
import { searchPageExists } from "../request/commons";
import { useToast } from "../hooks/useToast";
import { OverwriteBehaviourField } from "./OverwriteBehaviourField";
import { monoStack } from "../theme";
import { applyChartSourceToDescription } from "../utils";

interface ProcessingLink {
  url: string
  status: "pending" | "loading" | "done" | "failed"
}

export interface MultiImportModalProps {
  onAdd: (imports: MapImporterFormItem[]) => void
}

export function MultiImportModal({ onAdd }: MultiImportModalProps) {
  const [open, setOpen] = useState(false);
  const [links, setLinks] = useState("");
  const [processedLinks, setProcessedLinks] = useState<ProcessingLink[]>([]);
  const [loading, setLoading] = useState(false);
  const [descriptionOverwriteBehaviour, setDescriptionOverwriteBehaviour] = useState(DESCRIPTION_OVERWRITE_OPTIONS[0].value);
  const { showToast } = useToast();
  const handleOpen = useCallback(() => setOpen(true), [setOpen]);
  const handleClose = useCallback(() => {
    setOpen(false);
    setLinks("");
    setProcessedLinks([]);
    setLoading(false);
  }, [setOpen, setLinks, setProcessedLinks, setLoading]);

  const linksArray = useMemo(() => {
    if (!links.trim().length) {
      return []
    }
    return links.trim()
      .split("\n")
      .filter(l => l.trim())
  }, [links])

  const linksAreValid = useMemo(() => {
    if (linksArray.length) {
      return linksArray
        .every(l => l.startsWith(OWID_CHART_URL_PREFIX));
    }
    return true
  }, [linksArray])

  const disabled = useMemo(() => {
    return loading;
  }, [loading])


  const onSubmit = useCallback(() => {
    if (linksArray.length == 0) {
      return showToast("No links are submitted", "error");
    }

    setLoading(true);
    const limit = pLimit(2);
    setProcessedLinks(linksArray.map(url => ({ url, status: "pending" })));
    const input = linksArray.map(url =>
      limit(async () => {
        console.log("Starting: ", url)
        setProcessedLinks(old => {
          const urlIndex = old.findIndex(item => item.url == url);
          if (urlIndex != -1) {
            old[urlIndex].status = "loading";
          }
          return [...old];
        });
        try {

          const result = await getChartParameters(url);
          console.log("Done ", url)

          const imp = generateBlankImport();

          imp.linkVerified = true;
          imp.url = url;
          imp.descriptionOverwriteBehaviour = descriptionOverwriteBehaviour;

          if (result) {
            if (result.info.singleImage) {
              imp.fileName = INITIAL_FILENAME_MAP_SINGLE_IMAGE;
              imp.description = INITIAL_DESCRIPTION_MAP_SINGLE_IMAGE;
              imp.categories = INITIAL_CATEGORIES_MAP_SINGLE_IMAGE;
              imp.singleImage = true;
            } else {

              if (result.params && result.params.length > 0) {
                const paramsKeys = result.params.map(param => param.slug);
                const parts = url.split("?").pop()?.split("&")
                const selectedParams: SelectedParameter[] = []
                let newInitialFilenameMap = "$NAME";
                let newInitialFilenameChart = "$NAME";
                let newTemplateName = "$CHART_NAME"
                parts?.forEach(part => {
                  const [key, val] = part.split("=");
                  if (key && val && paramsKeys.includes(key)) {
                    const param = result.params.find(p => p.slug == key)
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

                imp.fileName = newInitialFilenameMap;
                imp.countryFileName = newInitialFilenameChart;
                imp.selectedChartParameters = selectedParams;
                imp.templateNameFormat = newTemplateName;
              }

              if (result.info?.title) {
                let templateName = `${COMMONS_TEMPLATE_PREFIX}/${imp.templateNameFormat}`;
                templateName = templateName.replace("$CHART_NAME", result.info?.title);
                if (imp.selectedChartParameters.length > 0) {
                  imp.selectedChartParameters.forEach((param) => {
                    templateName = templateName.replace(`$${param.key.toUpperCase()}`, param.valueName);
                  })
                }

                try {
                  const templateExists = await searchPageExists(templateName);
                  imp.templateExists = templateExists;
                } catch (err) {
                  console.log("Error checking if template exists: ", { url: imp.url, templateName, err });
                }
              }


            }
          }



          if (result?.info?.source) {
            imp.description = applyChartSourceToDescription(imp.description, result.info.source);
            imp.countryDescription = applyChartSourceToDescription(imp.countryDescription, result.info.source);
          }

          if (!result.error) {
            imp.canImport = true;
          }

          setProcessedLinks(old => {
            const urlIndex = old.findIndex(item => item.url == url);
            if (urlIndex != -1) {
              old[urlIndex].status = imp.canImport ? "done" : "failed";
            }
            return [...old];
          });

          return {
            url,
            result,
            imp
          }
        } catch (err) {
          console.log(err)

          setProcessedLinks(old => {
            const urlIndex = old.findIndex(item => item.url == url);
            if (urlIndex != -1) {
              old[urlIndex].status = "failed";
            }
            return [...old];
          });
          return null
        }


      }))
    Promise.all(input)
      .then(res => {
        const imports: MapImporterFormItem[] = res.filter(item => item).map(item => item!.imp).filter(item => item);
        onAdd(imports);
        handleClose()
      })
      .catch(err => {
        console.log("Error getting multi chart params: ", err);
        showToast("Failed to load chart parameters", "error");
      })
      .finally(() => {
        setLoading(false);
      })
  }, [descriptionOverwriteBehaviour, linksArray, setLoading, setProcessedLinks, onAdd, handleClose, showToast])

  return (
    <>
      <Button onClick={handleOpen} variant="outlined">Multi import</Button>
      <Dialog
        open={open}
        onClose={handleClose}
        fullWidth
        maxWidth="sm"
        aria-labelledby="multi-import-title"
      >
        <DialogTitle id="multi-import-title">Import multiple charts</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={3}>
            <TextField
              multiline
              minRows={6}
              maxRows={12}
              fullWidth
              disabled={disabled}
              label="Chart URLs"
              placeholder={`${OWID_CHART_URL_PREFIX}/grapher/life-expectancy\n${OWID_CHART_URL_PREFIX}/grapher/child-mortality`}
              helperText="One Our World in Data chart URL per line"
              value={links}
              onChange={(e) => setLinks(e.target.value)}
              sx={{
                mt: 1,
                '& .MuiInputBase-input': { fontFamily: monoStack, fontSize: 13 },
              }}
            />
            {!linksAreValid && (
              <Alert severity="error" variant="outlined">
                Some links are invalid — every line must start with {OWID_CHART_URL_PREFIX}
              </Alert>
            )}
            {processedLinks.length > 0 && (
              <Stack spacing={1} sx={{ maxHeight: 300, overflowY: "auto", overflowX: "clip" }}>
                {processedLinks.map((url, index) => (
                  <Stack key={`${url.url}-${index}`} direction="row" justifyContent="space-between" alignItems="center" spacing={1}>
                    <Link
                      href={url.url}
                      target="_blank"
                      noWrap
                      underline="hover"
                      sx={{ fontFamily: monoStack, fontSize: 13 }}
                    >
                      {url.url}
                    </Link>
                    {url.status == "done" && (
                      <CheckCircle color="success" fontSize="small" />
                    )}
                    {url.status == "loading" && (
                      <CircularProgress size={16} />
                    )}
                    {url.status == "failed" && (
                      <CloseIcon color="error" fontSize="small" />
                    )}
                  </Stack>
                ))}
              </Stack>
            )}
            {loading ? (
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                If a file with the same name exists: {DESCRIPTION_OVERWRITE_OPTIONS.filter(o => o.value === descriptionOverwriteBehaviour)[0]?.title}
              </Typography>
            ) : (
              <OverwriteBehaviourField
                options={DESCRIPTION_OVERWRITE_OPTIONS}
                value={descriptionOverwriteBehaviour}
                onChange={(val) => setDescriptionOverwriteBehaviour(val as typeof descriptionOverwriteBehaviour)}
                disabled={disabled}
              />
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose} disabled={disabled}>Cancel</Button>
          <Button disabled={disabled} loading={loading} variant="contained" onClick={onSubmit}>Submit</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
