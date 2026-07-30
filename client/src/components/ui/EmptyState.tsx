import { Box, Stack, Typography } from "@mui/material";
import { ReactNode } from "react";

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <Box
      sx={{
        border: '1px dashed',
        borderColor: 'divider',
        borderRadius: 1.5,
        py: 8,
        px: 3,
      }}
    >
      <Stack spacing={1.5} alignItems="center" textAlign="center">
        {icon && (
          <Box sx={{ color: 'text.secondary', '& svg': { fontSize: 40 } }}>
            {icon}
          </Box>
        )}
        <Typography sx={{ fontWeight: 600 }}>{title}</Typography>
        {description && (
          <Typography variant="body2" sx={{ color: 'text.secondary', maxWidth: 400 }}>
            {description}
          </Typography>
        )}
        {action}
      </Stack>
    </Box>
  )
}
