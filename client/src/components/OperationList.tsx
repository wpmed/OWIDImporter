import { Box, Button, ButtonGroup, Card, CardActionArea, CardActions, CardContent, Chip, Grid, Pagination, Stack, Typography } from "@mui/material"
import { useEffect, useState } from "react"
import { Operation } from "../types"
import { archiveOperation, fetchOperations } from "../request/request"
import { SocketMessage, SocketMessageActionEnum, useWebsocket } from "../hooks/useWebsocket"
import { formatDate, getOperationStatusColor } from "../utils"

interface OperationListProps {
  onOperationClick: (operation: Operation) => void
  onNew: () => void,
}

export function OperationList({ onOperationClick, onNew }: OperationListProps) {
  const [operations, setOperations] = useState<Operation[]>([]);
  const [archived, setArchived] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const perPage = 20;
  const { ws, connect, disconnect } = useWebsocket();

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
      })
  }

  useEffect(() => {
    fetchOperations({ archived, page, perPage })
      .then(res => {
        if (res.operations) {
          setOperations(res.operations);
        }
        setTotalPages(res.totalPages > 0 ? res.totalPages : 1);
      })
      .catch(err => {
        console.log({ err });
      })
  }, [archived, page, perPage])

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
    <Stack spacing={2} textAlign={"left"}>
      <Box>
        <Button sx={{ textTransform: "capitalize" }} variant="contained" onClick={onNew}>
          New Defaults Cleanup
        </Button>
      </Box>
      <hr />
      <Box>
        <ButtonGroup>
          <Button onClick={() => { setArchived(0); setPage(1); }} variant={archived == 0 ? "contained" : undefined}>Recent</Button>
          <Button onClick={() => { setArchived(1); setPage(1); }} variant={archived == 1 ? "contained" : undefined}>Archived</Button>
        </ButtonGroup>
      </Box>

      <Grid container spacing={4}>
        {operations.map(operation => (
          <Grid size={4} key={operation.id}>
            <Card>
              <CardActionArea onClick={() => onOperationClick(operation)}>
                <CardContent>
                  <Box display="flex" alignItems="center" justifyContent="space-between">
                    <Typography variant="subtitle1">Defaults Cleanup</Typography>
                    <Chip
                      size="small"
                      label={operation.status}
                      sx={{ backgroundColor: getOperationStatusColor(operation.status), color: "white", textTransform: "capitalize" }}
                    />
                  </Box>
                  <Typography variant="body2" color="text.secondary">
                    {formatDate(new Date(operation.createdAt * 1000))}
                  </Typography>
                </CardContent>
              </CardActionArea>
              <CardActions>
                <Button size="small" onClick={() => onToggleOperationArchived(operation)}>
                  {operation.archived === 0 ? "Archive" : "Unarchive"}
                </Button>
              </CardActions>
            </Card>
          </Grid>
        ))}
      </Grid>

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
