import { Stack, Typography } from "@mui/material";

export const Login = () => {
  const loginUrl = import.meta.env.VITE_BASE_URL + "/login";

  return (
    <Stack spacing={2}>
      <Typography variant="h3">OWID Importer</Typography>
      <Stack spacing={1}>
        <Typography>
          This is a tool to import freely licensed graphs from OurWorldInData into Wikimedia Commons.
        </Typography>
        <Typography>
          To Continue, Please <a href={loginUrl}>login.</a>
        </Typography>
      </Stack>
    </Stack>
  )
}
