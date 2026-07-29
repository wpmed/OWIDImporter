import { Alert, Box, Button, Chip, Stack, TextField, Typography } from "@mui/material"
import { useCallback, useEffect, useMemo, useState } from "react"
import { cancelOperation, createOperation, fetchOperationById, retryOperation } from "../request/request"
import { Operation, OperationItem, OperationItemStatusEnum, OperationStatusEnum, OperationTypeEnum } from "../types"
import { SocketMessage, SocketMessageActionEnum, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket"
import { getOperationItemStatusColor, getOperationStatusColor } from "../utils"

interface OperationRunnerProps {
  operationId: string
  onNavigateToList: () => void
}

export function OperationRunner({ operationId: initialOperationId, onNavigateToList }: OperationRunnerProps) {
  const [operationId, setOperationId] = useState(initialOperationId);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [items, setItems] = useState<OperationItem[]>([]);
  const [pagesInput, setPagesInput] = useState("");
  const [error, setError] = useState("");
  const [invalidPages, setInvalidPages] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const { ws, connect, disconnect } = useWebsocket();

  const getOperation = useCallback((id: string) => {
    fetchOperationById(id)
      .then(res => {
        if (res.operation) {
          setOperation(res.operation);
          setItems(res.items || []);
        }
      })
      .catch(err => {
        console.log({ err });
      })
  }, [setOperation, setItems])

  const onSubmit = () => {
    const pages = pagesInput.split("\n").map(p => p.trim()).filter(p => p !== "");
    if (pages.length === 0) {
      setError("Please enter at least one page");
      return;
    }
    setError("");
    setInvalidPages([]);
    setLoading(true);
    createOperation({ type: OperationTypeEnum.UPDATE_DEFAULTS, pages })
      .then(res => {
        if (res.operationId) {
          setOperationId(res.operationId);
        } else {
          setError(res.error || "Error creating operation");
          setInvalidPages(res.invalid || []);
        }
      })
      .catch(err => {
        console.log({ err });
        setError("Error creating operation");
      })
      .finally(() => {
        setLoading(false);
      })
  }

  const onCancel = () => {
    if (!operationId) return;
    cancelOperation(operationId)
      .then(() => getOperation(operationId))
      .catch(err => console.log({ err }))
  }

  const onRetry = () => {
    if (!operationId) return;
    retryOperation(operationId)
      .then(() => getOperation(operationId))
      .catch(err => console.log({ err }))
  }

  const doneItemsCount = useMemo(() => {
    return items.filter(item => [OperationItemStatusEnum.Updated, OperationItemStatusEnum.Skipped, OperationItemStatusEnum.Failed].includes(item.status)).length;
  }, [items])

  const failedItemsCount = useMemo(() => {
    return items.filter(item => item.status === OperationItemStatusEnum.Failed).length;
  }, [items])

  const canCancel = useMemo(() => {
    return operation && [OperationStatusEnum.Queued, OperationStatusEnum.Processing].includes(operation.status);
  }, [operation])

  const canRetry = useMemo(() => {
    if (!operation) return false;
    if ([OperationStatusEnum.Failed, OperationStatusEnum.Cancelled].includes(operation.status)) return true;
    return operation.status === OperationStatusEnum.Done && failedItemsCount > 0;
  }, [operation, failedItemsCount])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])

  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<string>) {
        const info = JSON.parse(event.data) as SocketMessage;
        switch (info.type) {
          case SocketMessageTypeEnum.OPERATION_ITEM: {
            const item = JSON.parse(info.msg) as OperationItem;
            setItems((items) => {
              const newItems = items.slice();
              const index = newItems.findIndex(i => i.id == item.id);
              if (index != -1) {
                newItems[index] = item;
              } else {
                newItems.push(item);
              }

              return newItems;
            })
            break;
          }
          case SocketMessageTypeEnum.OPERATION:
            setOperation(JSON.parse(info.msg) as Operation);
            break;
        }
      }

      ws.addEventListener("message", listener);
      return () => {
        ws.removeEventListener("message", listener)
      }
    }
  }, [ws, setItems, setOperation])

  useEffect(() => {
    if (ws && operationId) {
      ws.send(JSON.stringify({
        action: SocketMessageActionEnum.SUBSCRIBE_OPERATION,
        content: operationId
      }))

      return () => {
        ws.send(JSON.stringify({
          action: SocketMessageActionEnum.UNSUBSCRIBE_OPERATION,
          content: operationId
        }))
      }
    }
    return () => { }
  }, [ws, operationId])

  useEffect(() => {
    if (operationId) {
      getOperation(operationId);
    }
  }, [operationId, getOperation])

  if (!operationId) {
    return (
      <Stack spacing={2} textAlign={"left"}>
        <Typography variant="h5">Update Template Defaults</Typography>
        <Typography variant="body2">
          Resets the start, location, startingView and language parameters of each template page to their defaults.
        </Typography>
        {error ? (
          <Alert severity="error">
            {error}
            {invalidPages.length > 0 ? (
              <ul style={{ margin: 0 }}>
                {invalidPages.map(page => (
                  <li key={page}>{page}</li>
                ))}
              </ul>
            ) : null}
          </Alert>
        ) : null}
        <TextField
          multiline
          minRows={10}
          label="Pages"
          placeholder={"https://commons.wikimedia.org/wiki/Template:OWID/Example\nTemplate:OWID/Another_example"}
          helperText="One Commons page per line — full URL or page title"
          value={pagesInput}
          onChange={(e) => setPagesInput(e.target.value)}
        />
        <Box>
          <Button variant="contained" onClick={onSubmit} disabled={loading}>
            Start
          </Button>
          <Button sx={{ marginLeft: 2 }} onClick={onNavigateToList}>
            Back to list
          </Button>
        </Box>
      </Stack>
    )
  }

  return (
    <Stack spacing={2} textAlign={"left"}>
      <Typography variant="h5">Update Template Defaults</Typography>
      {operation ? (
        <Box display="flex" alignItems="center" gap={2}>
          <Chip
            label={operation.status}
            sx={{ backgroundColor: getOperationStatusColor(operation.status), color: "white", textTransform: "capitalize" }}
          />
          <Typography variant="body2">
            {doneItemsCount} of {items.length} pages processed
            {failedItemsCount > 0 ? ` — ${failedItemsCount} failed` : ""}
          </Typography>
        </Box>
      ) : null}
      <Box>
        {canCancel ? (
          <Button variant="outlined" color="error" onClick={onCancel} sx={{ marginRight: 2 }}>
            Cancel
          </Button>
        ) : null}
        {canRetry ? (
          <Button variant="outlined" onClick={onRetry} sx={{ marginRight: 2 }}>
            Retry Failed
          </Button>
        ) : null}
        <Button onClick={onNavigateToList}>
          Back to list
        </Button>
      </Box>
      <Stack spacing={1}>
        {items.map(item => (
          <Box key={item.id} display="flex" alignItems="center" gap={2} sx={{ borderBottom: "1px solid #eee", paddingBottom: 1 }}>
            <Chip
              size="small"
              label={item.status}
              sx={{ backgroundColor: getOperationItemStatusColor(item.status), color: "white", textTransform: "capitalize", minWidth: 90 }}
            />
            <a href={`${import.meta.env.VITE_MW_BASE_URL}/${item.title}`} target="_blank">
              <Typography variant="body2">{item.title}</Typography>
            </a>
            {item.error ? (
              <Typography variant="body2" color="error">{item.error}</Typography>
            ) : null}
          </Box>
        ))}
      </Stack>
    </Stack>
  )
}
