import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
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
import CodeIcon from '@mui/icons-material/Code';
import PostAddIcon from '@mui/icons-material/PostAdd';
import MenuIcon from '@mui/icons-material/Menu';
import AccountCircleIcon from '@mui/icons-material/AccountCircle';
import LoginIcon from '@mui/icons-material/Login';

import { ListItemIcon } from '@mui/material';
import { Link as RouterLink, Navigate, Outlet, Route, Routes, useLocation, useMatch } from 'react-router';
import { Login } from './components/Login';
import { useReplaceSession } from './hooks/useReplaceSession';
import { SESSION_ID_KEY, USERNAME_KEY } from './constants';
import { Logout } from '@mui/icons-material';
import { ReactNode, useState } from 'react';
import { logout } from './request/request';
import { serifStack } from './theme';
import { isNavActive, NavMatch, PATHS } from './routing';
import { LOGIN_URL, rememberCurrentPage } from './login';
import {
  IframeListPage,
  IframeTaskPage,
  MapListPage,
  MapTaskPage,
  NewIframePage,
  NewMapPage,
  NewOperationPage,
  NotFoundPage,
  OperationListPage,
  OperationPage,
} from './pages';

const drawerWidth = 240;

interface NavItem {
  to: string
  title: string
  icon: ReactNode
  match: NavMatch
}

interface NavSection {
  label: string
  items: NavItem[]
}

const NAV_SECTIONS: NavSection[] = [
  {
    label: "All Commons",
    items: [
      {
        to: PATHS.MAPS,
        title: "Map imports",
        icon: <MapIcon />,
        match: { list: PATHS.MAPS, newPath: PATHS.NEW_MAP },
      },
      {
        to: PATHS.NEW_MAP,
        title: "Import map",
        icon: <AddPhotoAlternateIcon />,
        match: { exact: PATHS.NEW_MAP },
      },
    ],
  },
  {
    label: "Iframe",
    items: [
      {
        to: PATHS.IFRAMES,
        title: "Iframe imports",
        icon: <CodeIcon />,
        match: { list: PATHS.IFRAMES, newPath: PATHS.NEW_IFRAME },
      },
      {
        to: PATHS.NEW_IFRAME,
        title: "Import iframe",
        icon: <PostAddIcon />,
        match: { exact: PATHS.NEW_IFRAME },
      },
    ],
  },
  {
    label: "Maintenance",
    items: [
      {
        to: PATHS.OPERATIONS,
        title: "Defaults cleanup",
        icon: <CleaningServicesIcon />,
        // Covers the list, /operations/new and /operations/:id
        match: { list: PATHS.OPERATIONS, newPath: "" },
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

const isLoggedIn = () => !!window.localStorage.getItem(SESSION_ID_KEY);

// Private pages show the login screen to anonymous visitors (the URL is kept,
// so logging in brings them back here).
function RequireAuth() {
  return isLoggedIn() ? <Outlet /> : <Login />
}

// True on the public import pages (/maps/:id, /iframes/:id), not on the new-import forms
function useOnPublicPage() {
  const mapMatch = useMatch("/maps/:id");
  const iframeMatch = useMatch("/iframes/:id");
  return [mapMatch, iframeMatch].some(m => m && m.params.id !== "new");
}

function Layout() {
  const [mobileOpen, setMobileOpen] = useState(false);
  const { pathname } = useLocation();
  const onPublicPage = useOnPublicPage();

  const sessionId = window.localStorage.getItem(SESSION_ID_KEY);
  const username = window.localStorage.getItem(USERNAME_KEY);

  const onLogout = () => {
    logout().finally(() => {
      window.localStorage.removeItem(SESSION_ID_KEY);
      window.localStorage.removeItem(USERNAME_KEY);
      window.location.reload();
    })
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
              <ListItem key={item.to} disablePadding>
                <ListItemButton
                  component={RouterLink}
                  to={item.to}
                  onClick={() => setMobileOpen(false)}
                  selected={isNavActive(item.match, pathname)}
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
          {!sessionId && onPublicPage && (
            <Button
              variant="outlined"
              href={LOGIN_URL}
              onClick={rememberCurrentPage}
              startIcon={<LoginIcon />}
            >
              Log in
            </Button>
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
              <Outlet />
            </Box>
          </Box>
        </>
      ) : (
        <Box component="main" sx={{ flexGrow: 1, p: { xs: 2, md: 4 }, minWidth: 0 }}>
          <Toolbar />
          <Box sx={{ maxWidth: 1400, mx: 'auto' }}>
            <Outlet />
          </Box>
        </Box>
      )}
    </Box>
  );
}

export default function App() {
  useReplaceSession();

  return (
    <Routes>
      <Route element={<Layout />}>
        {/* Public: a single import, read-only unless the viewer created it */}
        <Route path="/maps/:id" element={<MapTaskPage />} />
        <Route path="/iframes/:id" element={<IframeTaskPage />} />

        <Route element={<RequireAuth />}>
          <Route index element={<Navigate to={PATHS.MAPS} replace />} />
          <Route path={PATHS.MAPS} element={<MapListPage />} />
          <Route path={PATHS.NEW_MAP} element={<NewMapPage />} />
          <Route path={PATHS.IFRAMES} element={<IframeListPage />} />
          <Route path={PATHS.NEW_IFRAME} element={<NewIframePage />} />
          <Route path={PATHS.OPERATIONS} element={<OperationListPage />} />
          <Route path={PATHS.NEW_OPERATION} element={<NewOperationPage />} />
          <Route path="/operations/:id" element={<OperationPage />} />
        </Route>

        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
