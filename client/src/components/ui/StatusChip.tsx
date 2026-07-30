import { Box, CircularProgress, Stack, Typography } from "@mui/material";
import { alpha } from "@mui/material/styles";
import { StatusKind } from "../../theme";

interface StatusChipProps {
  kind: StatusKind
  label: string
  showSpinner?: boolean
  size?: "small" | "medium"
}

export function StatusChip({ kind, label, showSpinner, size = "medium" }: StatusChipProps) {
  const small = size === "small";

  return (
    <Stack
      direction="row"
      alignItems="center"
      spacing={small ? 0.6 : 0.8}
      sx={{
        display: "inline-flex",
        px: small ? 1 : 1.25,
        py: small ? 0.25 : 0.4,
        borderRadius: 99,
        backgroundColor: (t) => alpha(t.palette.status[kind], 0.1),
      }}
    >
      {showSpinner ? (
        <CircularProgress
          size={small ? 8 : 10}
          thickness={6}
          sx={{ color: (t) => t.palette.status[kind] }}
        />
      ) : (
        <Box
          sx={{
            width: 6,
            height: 6,
            borderRadius: "50%",
            backgroundColor: (t) => t.palette.status[kind],
          }}
        />
      )}
      <Typography
        component="span"
        sx={{
          fontSize: small ? 11 : 12,
          fontWeight: 600,
          lineHeight: 1.6,
          textTransform: "capitalize",
          color: (t) => t.palette.status[kind],
        }}
      >
        {label}
      </Typography>
    </Stack>
  )
}
