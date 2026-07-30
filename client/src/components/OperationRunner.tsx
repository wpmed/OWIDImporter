import { Alert, Button, LinearProgress, Link, Stack, TextField, Typography } from "@mui/material"
import { useCallback, useEffect, useMemo, useState } from "react"
import { cancelOperation, createOperation, fetchOperationById, retryOperation } from "../request/request"
import { Operation, OperationItem, OperationItemStatusEnum, OperationStatusEnum, OperationTypeEnum } from "../types"
import { SocketMessage, SocketMessageActionEnum, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket"
import { getOperationItemStatusKind, getOperationStatusKind } from "../utils"
import { StatusChip } from "./ui/StatusChip"
import { useToast } from "../hooks/useToast"
import { PageHeader } from "./ui/PageHeader"
import { monoStack } from "../theme"

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
  const { showToast } = useToast();

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
      .catch(err => {
        console.log({ err })
        showToast("Failed to cancel the operation", "error")
      })
  }

  const onRetry = () => {
    if (!operationId) return;
    retryOperation(operationId)
      .then(() => getOperation(operationId))
      .catch(err => {
        console.log({ err })
        showToast("Failed to retry the operation", "error")
      })
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
      <Stack spacing={3}>
        <PageHeader
          title="New defaults cleanup"
          subtitle="Resets the start, location, startingView and language parameters of each template page to their defaults"
        />
        {error ? (
          <Alert severity="error" variant="outlined">
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
          sx={{ '& .MuiInputBase-input': { fontFamily: monoStack, fontSize: 13 } }}
        />
        <Stack direction="row" spacing={2}>
          <Button variant="contained" onClick={onSubmit} disabled={loading}>
            Start
          </Button>
          <Button onClick={onNavigateToList}>
            Back to list
          </Button>
        </Stack>
      </Stack>
    )
  }

  return (
    <Stack spacing={3}>
      <PageHeader
        title="Defaults cleanup"
        subtitle="Live progress for this cleanup run"
        action={(
          <Stack direction="row" spacing={2}>
            {canCancel ? (
              <Button variant="outlined" color="error" onClick={onCancel}>
                Cancel
              </Button>
            ) : null}
            {canRetry ? (
              <Button variant="outlined" color="warning" onClick={onRetry}>
                Retry failed
              </Button>
            ) : null}
            <Button onClick={onNavigateToList}>
              Back to list
            </Button>
          </Stack>
        )}
      />
      {operation ? (
        <Stack spacing={1.5}>
          <Stack direction="row" alignItems="center" spacing={2}>
            <StatusChip
              label={operation.status}
              kind={getOperationStatusKind(operation.status)}
              showSpinner={operation.status === OperationStatusEnum.Processing}
            />
            <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: monoStack }}>
              {doneItemsCount}/{items.length} pages
              {failedItemsCount > 0 ? ` · ${failedItemsCount} failed` : ""}
            </Typography>
          </Stack>
          {items.length > 0 && (
            <LinearProgress
              variant="determinate"
              value={(doneItemsCount / items.length) * 100}
              sx={{ borderRadius: 99, height: 6 }}
            />
          )}
        </Stack>
      ) : null}
      <Stack>
        {items.map(item => (
          <Stack
            key={item.id}
            direction="row"
            alignItems="center"
            spacing={1.5}
            sx={{ borderBottom: '1px solid', borderColor: 'divider', py: 1 }}
          >
            <StatusChip
              size="small"
              label={item.status}
              kind={getOperationItemStatusKind(item.status)}
            />
            <Link
              href={`${import.meta.env.VITE_MW_BASE_URL}/${item.title}`}
              target="_blank"
              underline="hover"
              noWrap
              sx={{ fontFamily: monoStack, fontSize: 13 }}
            >
              {item.title}
            </Link>
            {item.error ? (
              <Typography variant="caption" color="error" sx={{ minWidth: 0 }}>{item.error}</Typography>
            ) : null}
          </Stack>
        ))}
      </Stack>
    </Stack>
  )
}
