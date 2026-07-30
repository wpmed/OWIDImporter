import { Box, Button, Card, CardContent, Divider, Grid, Pagination, Stack, ToggleButton, ToggleButtonGroup, Typography } from "@mui/material"
import { useEffect, useState } from "react"
import { Operation, OperationStatusEnum } from "../types"
import { archiveOperation, fetchOperations } from "../request/request"
import { SocketMessage, SocketMessageActionEnum, useWebsocket } from "../hooks/useWebsocket"
import { formatDate, getOperationStatusKind } from "../utils"
import { StatusChip } from "./ui/StatusChip"
import { useToast } from "../hooks/useToast"
import { PageHeader } from "./ui/PageHeader"
import { EmptyState } from "./ui/EmptyState"
import { TaskCardSkeleton } from "./ui/TaskCardSkeleton"
import { Add, CleaningServices } from "@mui/icons-material"

interface OperationListProps {
  onOperationClick: (operation: Operation) => void
  onNew: () => void,
}

export function OperationList({ onOperationClick, onNew }: OperationListProps) {
  const [operations, setOperations] = useState<Operation[]>([]);
  const [loading, setLoading] = useState(true);
  const [archived, setArchived] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const perPage = 20;
  const { ws, connect, disconnect } = useWebsocket();
  const { showToast } = useToast();

  const onToggleOperationArchived = (operation: Operation) => {
    const newArchived = operation.archived === 0 ? 1 : 0;

    archiveOperation(operation.id, newArchived)
      .then((res) => {
        if (res.operation) {
          setOperations((operations) => {
            return operations.slice().filter(o => o.id != operation.id);
          });
        }
      })
      .catch(err => {
        console.log("Error updating archive: ", newArchived, err)
        showToast("Failed to update the operation", "error")
      })
  }

  useEffect(() => {
    setLoading(true);
    fetchOperations({ archived, page, perPage })
      .then(res => {
        if (res.operations) {
          setOperations(res.operations);
        }
        setTotalPages(res.totalPages > 0 ? res.totalPages : 1);
      })
      .catch(err => {
        console.log({ err });
        showToast("Failed to load operations", "error")
      })
      .finally(() => {
        setLoading(false);
      })
  }, [archived, page, perPage, showToast])

  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<string>) {
        const info = JSON.parse(event.data) as SocketMessage;
        const operation = JSON.parse(info.msg) as Operation;
        setOperations((operations) => {
          const newOperations = operations.slice();
          const index = newOperations.findIndex(o => o.id === operation.id)
          if (index !== -1) {
            newOperations[index] = operation;
          }

          return newOperations;
        })
      }

      ws.addEventListener("message", listener);
      return () => {
        ws.removeEventListener("message", listener)
      }
    }

    return () => { }
  }, [ws, setOperations])

  useEffect(() => {
    if (ws) {
      ws.send(JSON.stringify({
        action: SocketMessageActionEnum.SUBSCRIBE_OPERATION_LIST,
      }))

      return () => {
        ws.send(JSON.stringify({
          action: SocketMessageActionEnum.UNSUBSCRIBE_OPERATION_LIST,
        }))
      }
    }
    return () => { }
  }, [ws])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])

  return (
    <Stack spacing={3}>
      <PageHeader
        title="Defaults cleanup"
        subtitle="Reset OWID template parameters on Commons pages back to their defaults"
        action={(
          <Button variant="contained" startIcon={<Add />} onClick={onNew}>
            New cleanup
          </Button>
        )}
      />

      <Box>
        <ToggleButtonGroup
          exclusive
          size="small"
          value={archived}
          onChange={(_, value) => {
            if (value !== null) {
              setArchived(value);
              setPage(1);
            }
          }}
        >
          <ToggleButton value={0}>Recent</ToggleButton>
          <ToggleButton value={1}>Archived</ToggleButton>
        </ToggleButtonGroup>
      </Box>

      {loading && operations.length === 0 ? (
        <Grid container spacing={3}>
          {Array.from({ length: 3 }, (_, i) => (
            <Grid size={{ xs: 12, sm: 6, lg: 4 }} key={i}>
              <TaskCardSkeleton />
            </Grid>
          ))}
        </Grid>
      ) : operations.length === 0 ? (
        <EmptyState
          icon={<CleaningServices />}
          title={archived ? "No archived cleanups" : "No cleanups yet"}
          description={archived
            ? "Cleanup runs you archive will show up here."
            : "Start a cleanup run to reset template defaults across Commons pages."}
          action={!archived && (
            <Button variant="contained" startIcon={<Add />} onClick={onNew}>
              New cleanup
            </Button>
          )}
        />
      ) : (
        <Grid container spacing={3}>
          {operations.map(operation => (
            <Grid size={{ xs: 12, sm: 6, lg: 4 }} key={operation.id}>
              <Card
                role="button"
                tabIndex={0}
                sx={{ height: '100%', cursor: 'pointer' }}
                onClick={() => onOperationClick(operation)}
                onKeyDown={(e) => {
                  if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) {
                    e.preventDefault();
                    onOperationClick(operation);
                  }
                }}
              >
                <CardContent>
                  <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1} sx={{ mb: 1 }}>
                    <Typography sx={{ fontWeight: 600 }}>Defaults cleanup</Typography>
                    <StatusChip
                      size="small"
                      label={operation.status}
                      kind={getOperationStatusKind(operation.status)}
                      showSpinner={operation.status === OperationStatusEnum.Processing}
                    />
                  </Stack>
                  <Divider sx={{ mb: 1 }} />
                  <Stack direction="row" justifyContent="space-between" alignItems="center" onClick={(e) => e.stopPropagation()}>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                      {formatDate(new Date(operation.createdAt * 1000))}
                    </Typography>
                    <Button size="small" onClick={() => onToggleOperationArchived(operation)}>
                      {operation.archived === 0 ? "Archive" : "Unarchive"}
                    </Button>
                  </Stack>
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      {totalPages > 1 && (
        <Box display="flex" justifyContent="center" mt={2}>
          <Pagination
            count={totalPages}
            page={page}
            onChange={(_, value) => setPage(value)}
            color="primary"
          />
        </Box>
      )}
    </Stack>
  )
}
