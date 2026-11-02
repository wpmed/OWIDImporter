import { Button, Stack } from "@mui/material";
import SearchOffIcon from '@mui/icons-material/SearchOff';
import { Link as RouterLink, useLocation, useNavigate, useParams } from "react-router";
import { useCallback } from "react";
import { TaskTypeEnum } from "./types";
import { TaskList } from "./components/TaskList";
import { MapImporter } from "./components/MapImporter";
import { IframeImporter } from "./components/IframeImporter";
import { IframeTaskResults } from "./components/IframeTaskResults";
import { OperationList } from "./components/OperationList";
import { OperationRunner } from "./components/OperationRunner";
import { ShareLinkButton } from "./components/ShareLinkButton";
import { PageHeader } from "./components/ui/PageHeader";
import { EmptyState } from "./components/ui/EmptyState";
import { iframePath, mapPath, operationPath, PATHS } from "./routing";

const scrollTop = () => window.scrollTo({ left: 0, top: 0 });

function useGoTo(path: string) {
  const navigate = useNavigate();
  return useCallback(() => {
    navigate(path);
    scrollTop();
  }, [navigate, path]);
}

export function MapListPage() {
  const navigate = useNavigate();
  return (
    <TaskList
      key={TaskTypeEnum.MAP}
      taskType={TaskTypeEnum.MAP}
      onNew={() => navigate(PATHS.NEW_MAP)}
      onTaskClick={(task) => navigate(mapPath(task.id))}
    />
  )
}

export function NewMapPage() {
  const navigate = useNavigate();
  // A new location key (sidebar click while already here) starts a blank form
  const { key } = useLocation();
  const toList = useGoTo(PATHS.MAPS);
  return (
    <MapImporter
      key={key}
      onNavigateToList={toList}
      onCreated={(id) => navigate(mapPath(id))}
    />
  )
}

// Public: anyone with the link sees the import; only its creator gets controls
export function MapTaskPage() {
  const { id = "" } = useParams();
  const toList = useGoTo(PATHS.MAPS);
  return <MapImporter key={id} taskId={id} onNavigateToList={toList} />
}

export function IframeListPage() {
  const navigate = useNavigate();
  return (
    <TaskList
      key={TaskTypeEnum.IFRAME}
      taskType={TaskTypeEnum.IFRAME}
      onNew={() => navigate(PATHS.NEW_IFRAME)}
      onTaskClick={(task) => navigate(iframePath(task.id))}
    />
  )
}

export function NewIframePage() {
  const navigate = useNavigate();
  const { key } = useLocation();
  return <IframeImporter key={key} onCreated={(id) => navigate(iframePath(id))} />
}

// Public: anyone with the link sees the import; only its creator gets controls
export function IframeTaskPage() {
  const { id = "" } = useParams();
  return (
    <Stack spacing={3}>
      <PageHeader
        title="Iframe import"
        subtitle="Live progress and {{OWID}} wikitext for this import"
        action={<ShareLinkButton />}
      />
      <IframeTaskResults key={id} taskIds={[id]} />
    </Stack>
  )
}

export function OperationListPage() {
  const navigate = useNavigate();
  return (
    <OperationList
      onNew={() => navigate(PATHS.NEW_OPERATION)}
      onOperationClick={(operation) => navigate(operationPath(operation.id))}
    />
  )
}

export function NewOperationPage() {
  const navigate = useNavigate();
  const { key } = useLocation();
  const toList = useGoTo(PATHS.OPERATIONS);
  return (
    <OperationRunner
      key={key}
      operationId=""
      onNavigateToList={toList}
      onCreated={(id) => navigate(operationPath(id))}
    />
  )
}

export function OperationPage() {
  const { id = "" } = useParams();
  const toList = useGoTo(PATHS.OPERATIONS);
  return <OperationRunner key={id} operationId={id} onNavigateToList={toList} />
}

export function NotFoundPage() {
  return (
    <Stack sx={{ pt: { xs: 2, md: 6 } }}>
      <EmptyState
        icon={<SearchOffIcon />}
        title="Page not found"
        description="This address does not match any page of the importer."
        action={<Button component={RouterLink} to="/" variant="contained">Go to the start page</Button>}
      />
    </Stack>
  )
}
