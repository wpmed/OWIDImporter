import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import IconButton from '@mui/material/IconButton';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import MapIcon from '@mui/icons-material/Map';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import CleaningServicesIcon from '@mui/icons-material/CleaningServices';
import MenuIcon from '@mui/icons-material/Menu';
import AccountCircleIcon from '@mui/icons-material/AccountCircle';

import { ListItemIcon } from '@mui/material';
import { MapImporter } from './components/MapImporter';
import { Login } from './components/Login';
import { useReplaceSession } from './hooks/useReplaceSession';
import { SESSION_ID_KEY, USERNAME_KEY } from './constants';
import { Logout } from '@mui/icons-material';
import { ReactNode, useCallback, useMemo, useState } from 'react';
import { Operation, Task, TaskTypeEnum } from './types';
import { logout } from './request/request';
import { TaskList } from './components/TaskList';
import { OperationList } from './components/OperationList';
import { OperationRunner } from './components/OperationRunner';
import { serifStack } from './theme';

const drawerWidth = 240;

enum TABS {
  MAP_LIST = 0,
  MAP_DETAILS = 1,
  CHART_LIST = 2,
  CHART_DETAILS = 3,
  IMPORT_MAP = 4,
  OPERATION_LIST = 6,
  OPERATION_DETAILS = 7,
  NEW_OPERATION = 8,
}

interface NavItem {
  id: TABS
  title: string
  icon: ReactNode
  match: TABS[]
}

interface NavSection {
  label: string
  items: NavItem[]
}

const NAV_SECTIONS: NavSection[] = [
  {
    label: "Import",
    items: [
      {
        id: TABS.MAP_LIST,
        title: "Map imports",
        icon: <MapIcon />,
        match: [TABS.MAP_LIST, TABS.MAP_DETAILS],
      },
      {
        id: TABS.IMPORT_MAP,
        title: "Import map",
        icon: <AddPhotoAlternateIcon />,
        match: [TABS.IMPORT_MAP],
      },
    ],
  },
  {
    label: "Maintenance",
    items: [
      {
        id: TABS.OPERATION_LIST,
        title: "Defaults cleanup",
        icon: <CleaningServicesIcon />,
        match: [TABS.OPERATION_LIST, TABS.OPERATION_DETAILS, TABS.NEW_OPERATION],
      },
    ],
  },
];

function Brand() {
  return (
    <Stack direction="row" alignItems="baseline" spacing={1.5}>
      <Stack direction="row" spacing={0.5} sx={{ alignSelf: 'center' }}>
        <Box sx={{ width: 10, height: 10, borderRadius: 0.5, bgcolor: 'primary.main' }} />
        <Box sx={{ width: 10, height: 10, borderRadius: 0.5, bgcolor: 'secondary.main' }} />
      </Stack>
      <Typography
        noWrap
        component="div"
        sx={{ fontFamily: serifStack, fontWeight: 600, fontSize: 22, lineHeight: 1 }}
      >
        OWID Importer
      </Typography>
      <Typography
        variant="overline"
        noWrap
        sx={{ color: 'text.secondary', display: { xs: 'none', sm: 'block' }, lineHeight: 1 }}
      >
        for Wikimedia Commons
      </Typography>
    </Stack>
  )
}

export default function App() {
  const [tab, setTab] = useState(TABS.MAP_LIST);
  const [selectedTaskId, setSelectedTaskId] = useState("");
  const [selectedOperationId, setSelectedOperationId] = useState("");
  const [importerKey, setImporterKey] = useState(0);
  const [operationKey, setOperationKey] = useState(0);
  const [mobileOpen, setMobileOpen] = useState(false);

  useReplaceSession();

  const sessionId = window.localStorage.getItem(SESSION_ID_KEY);
  const username = window.localStorage.getItem(USERNAME_KEY);

  const selectedTaskType = useMemo(() => {
    if ([TABS.MAP_LIST, TABS.MAP_DETAILS].includes(tab)) {
      return TaskTypeEnum.MAP
    }
    return TaskTypeEnum.CHART
  }, [tab])

  const onLogout = () => {
    logout().finally(() => {
      window.localStorage.removeItem(SESSION_ID_KEY);
      window.localStorage.removeItem(USERNAME_KEY);
      window.location.reload();
    })
  }

  const onNewClick = () => {
    setSelectedTaskId("");
    setImporterKey((key) => key + 1);
    setTab(TABS.IMPORT_MAP);
  }

  const onNewOperationClick = () => {
    setSelectedOperationId("");
    setOperationKey((key) => key + 1);
    setTab(TABS.NEW_OPERATION);
  }

  const onOperationClick = (operation: Operation) => {
    setSelectedOperationId(operation.id);
    setTab(TABS.OPERATION_DETAILS);
  }

  const onNavigateToOperationList = useCallback(() => {
    setSelectedOperationId("");
    setTab(TABS.OPERATION_LIST);
    window.scrollTo({ left: 0, top: 0 })
  }, [setTab])

  const onTaskClick = (task: Task) => {
    setSelectedTaskId(task.id);
    if (selectedTaskType === TaskTypeEnum.MAP) {
      setTab(TABS.MAP_DETAILS);
    } else if (selectedTaskType == TaskTypeEnum.CHART) {
      setTab(TABS.CHART_DETAILS);
    }
  }

  const onNavigateToList = useCallback(() => {
    setTab(TABS.MAP_LIST);
    window.scrollTo({ left: 0, top: 0 })
  }, [setTab])

  const onNavItemClick = (item: NavItem) => {
    setMobileOpen(false);
    if (item.id === TABS.IMPORT_MAP) {
      onNewClick()
    } else {
      setSelectedTaskId("");
      setSelectedOperationId("");
      setTab(item.id);
    }
  }

  const drawerContent = (
    <Box sx={{ overflow: 'auto', display: 'flex', flexDirection: 'column', height: '100%' }}>
      {NAV_SECTIONS.map(section => (
        <Box key={section.label}>
          <Typography
            variant="overline"
            sx={{ px: 3, pt: 2, pb: 0.5, display: 'block', color: 'text.secondary', fontSize: 11 }}
          >
            {section.label}
          </Typography>
          <List disablePadding>
            {section.items.map(item => (
              <ListItem key={item.id} disablePadding>
                <ListItemButton
                  onClick={() => onNavItemClick(item)}
                  selected={item.match.includes(tab)}
                >
                  <ListItemIcon>
                    {item.icon}
                  </ListItemIcon>
                  <ListItemText primary={item.title} />
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Box>
      ))}
      <List sx={{ mt: 'auto', borderTop: 1, borderColor: 'divider' }}>
        <ListItem disablePadding>
          <ListItemButton onClick={onLogout}>
            <ListItemIcon>
              <Logout sx={{ transform: "rotate(180deg)" }} />
            </ListItemIcon>
            <ListItemText primary={"Logout"} secondary={username} />
          </ListItemButton>
        </ListItem>
      </List>
    </Box>
  )

  return (
    <Box sx={{ display: 'flex' }}>
      <AppBar position="fixed" sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
        <Toolbar sx={{ gap: 1 }}>
          {sessionId && (
            <IconButton
              edge="start"
              aria-label="Open navigation"
              onClick={() => setMobileOpen(true)}
              sx={{ display: { md: 'none' } }}
            >
              <MenuIcon />
            </IconButton>
          )}
          <Brand />
          <Box sx={{ flexGrow: 1 }} />
          {username && (
            <Chip
              icon={<AccountCircleIcon />}
              label={username}
              variant="outlined"
              sx={{ display: { xs: 'none', sm: 'flex' } }}
            />
          )}
        </Toolbar>
      </AppBar>
      {sessionId ? (
        <>
          <Drawer
            variant="permanent"
            sx={{
              width: drawerWidth,
              flexShrink: 0,
              display: { xs: 'none', md: 'block' },
              [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: 'border-box' },
            }}
          >
            <Toolbar />
            {drawerContent}
          </Drawer>
          <Drawer
            variant="temporary"
            open={mobileOpen}
            onClose={() => setMobileOpen(false)}
            ModalProps={{ keepMounted: true }}
            sx={{
              display: { xs: 'block', md: 'none' },
              [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: 'border-box', bgcolor: 'background.default' },
            }}
          >
            <Toolbar />
            {drawerContent}
          </Drawer>
          <Box component="main" sx={{ flexGrow: 1, p: { xs: 2, md: 4 }, minWidth: 0 }}>
            <Toolbar />
            <Box sx={{ maxWidth: 1400, mx: 'auto' }}>
              {[TABS.MAP_LIST, TABS.CHART_LIST].includes(tab) ? (
                <TaskList taskType={selectedTaskType} onNew={onNewClick} onTaskClick={onTaskClick} />
              ) : [TABS.IMPORT_MAP, TABS.MAP_DETAILS].includes(tab) ? (
                <MapImporter
                  key={`${importerKey}-${selectedTaskId}`}
                  taskId={selectedTaskId}
                  onNavigateToList={onNavigateToList}
                />
              ) : tab === TABS.OPERATION_LIST ? (
                <OperationList onNew={onNewOperationClick} onOperationClick={onOperationClick} />
              ) : [TABS.NEW_OPERATION, TABS.OPERATION_DETAILS].includes(tab) ? (
                <OperationRunner
                  key={`${operationKey}-${selectedOperationId}`}
                  operationId={selectedOperationId}
                  onNavigateToList={onNavigateToOperationList}
                />
              ) : null}
            </Box>
          </Box>
        </>
      ) : (
        <Box component="main" sx={{ flexGrow: 1, p: { xs: 2, md: 4 } }}>
          <Toolbar />
          <Login />
        </Box>
      )}
    </Box>
  );
}
