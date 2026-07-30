import { Box, Button, Divider, Stack, Typography } from "@mui/material";
import LoginIcon from '@mui/icons-material/Login';

const STEPS = [
  {
    title: "Paste a chart URL",
    description: "Point the importer at any Our World in Data map or chart.",
  },
  {
    title: "Review the details",
    description: "Adjust the file name, description, and categories before upload.",
  },
  {
    title: "Upload to Commons",
    description: "Maps are rendered per region and year, then uploaded for you.",
  },
];

export const Login = () => {
  const loginUrl = import.meta.env.VITE_BASE_URL + "/login";

  return (
    <Stack alignItems="center" sx={{ pt: { xs: 4, md: 10 } }}>
      <Stack spacing={4} alignItems="center" sx={{ maxWidth: 720, textAlign: "center" }}>
        <Stack spacing={0.5} sx={{ width: 72 }}>
          <Box sx={{ height: 3, bgcolor: 'secondary.main' }} />
          <Box sx={{ height: 3, bgcolor: 'primary.main' }} />
        </Stack>
        <Typography variant="h2" sx={{ fontSize: { xs: 34, md: 48 } }}>
          Bring Our World in Data to Wikimedia Commons
        </Typography>
        <Typography sx={{ color: 'text.secondary', maxWidth: 560 }}>
          This is a tool to import freely licensed graphs from OurWorldInData
          into Wikimedia Commons — with the file descriptions, categories, and
          gallery templates handled for you.
        </Typography>
        <Button
          variant="contained"
          size="large"
          href={loginUrl}
          startIcon={<LoginIcon />}
          sx={{ px: 4 }}
        >
          Log in with Wikimedia
        </Button>
        <Divider flexItem />
        <Stack
          direction={{ xs: 'column', sm: 'row' }}
          spacing={{ xs: 3, sm: 4 }}
          divider={<Divider orientation="vertical" flexItem sx={{ display: { xs: 'none', sm: 'block' } }} />}
        >
          {STEPS.map((step, index) => (
            <Stack key={step.title} spacing={1} sx={{ flex: 1, textAlign: 'left' }}>
              <Typography variant="overline" sx={{ color: 'secondary.main' }}>
                Step {index + 1}
              </Typography>
              <Typography sx={{ fontWeight: 600 }}>{step.title}</Typography>
              <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {step.description}
              </Typography>
            </Stack>
          ))}
        </Stack>
      </Stack>
    </Stack>
  )
}
