import { Box, Button, CircularProgress, Modal, Stack, Typography } from "@mui/material"
import { useCallback, useMemo, useState } from "react";
import { COMMONS_TEMPLATE_PREFIX, generateBlankImport, OWID_CHART_URL_PREFIX } from "../constants";
import { getChartParameters } from "../request/request";
import pLimit from 'p-limit';
import { CheckCircle, Close as CloseIcon } from "@mui/icons-material";
import { MapImporterFormItem, SelectedParameter } from "../types";
import { searchPageExists } from "../request/commons";

const style = {
  position: 'absolute',
  top: '50%',
  left: '50%',
  transform: 'translate(-50%, -50%)',
  width: 500,
  bgcolor: 'background.paper',
  border: '1px solid #000',
  borderRadius: 2,
  boxShadow: 24,
  p: 4,
  pt: 2,
};

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
      return alert("No links are submitted");
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

          if (result && result.params && result.params.length > 0) {
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
            newInitialFilenameChart += ", $START_YEAR to $END_YEAR, $REGION.svg";

            imp.fileName = newInitialFilenameMap;
            imp.countryFileName = newInitialFilenameChart;
            imp.selectedChartParameters = selectedParams;
            imp.templateNameFormat = newTemplateName;
          }

          if (result && result.info?.title) {
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

          setProcessedLinks(old => {
            const urlIndex = old.findIndex(item => item.url == url);
            if (urlIndex != -1) {
              old[urlIndex].status = "done";
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
        console.log("Got multi res: ", res)
        const imports: MapImporterFormItem[] = res.filter(item => item).map(item => item!.imp).filter(item => item);
        console.log({ res, imports });
        onAdd(imports);
        handleClose()
      })
      .catch(err => {
        console.log("Error getting multi chart params: ", err);
      })
      .finally(() => {
        setLoading(false);
      })
  }, [linksArray, setLoading, setProcessedLinks, onAdd, handleClose])

  return (
    <>
      <Button onClick={handleOpen} variant="contained" >Multi Import</Button>
      <Modal
        open={open}
        onClose={handleClose}
        aria-labelledby="modal-modal-title"
        aria-describedby="modal-modal-description"
      >
        <Box sx={style}>
          <Stack spacing={4}>
            <Stack spacing={2}>
              <Typography variant="h5" component="h2">
                Add multiple links, one link per line
              </Typography>
              <textarea disabled={disabled} style={{ width: "100%", height: "200px", backgroundColor: "white", color: "black" }} value={links} onChange={(e) => setLinks(e.target.value)} />
              {!linksAreValid && (
                <Typography color="error">
                  Some links are invalid.
                </Typography>
              )}
              <Stack spacing={1}>
                {processedLinks.map(url => (
                  <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1}>
                    <Typography color={url.status == "done" ? "success" : "primary"} noWrap>
                      {url.url}
                    </Typography>
                    {url.status == "done" && (
                      <CheckCircle color="success" />

                    )}
                    {url.status == "loading" && (
                      <CircularProgress size={20} />
                    )}
                    {url.status == "failed" && (
                      <CloseIcon color="error" />
                    )}
                  </Stack>
                ))}
              </Stack>
            </Stack>
            <Stack justifyContent="flex-end" flexDirection="row">
              <Button onClick={handleClose} disabled={disabled}>Cancel</Button>
              <Button disabled={disabled} loading={loading} variant="contained" onClick={onSubmit}>Submit</Button>
            </Stack>
          </Stack>
        </Box>
      </Modal>
    </>
  )
}
