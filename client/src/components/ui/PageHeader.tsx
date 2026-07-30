import { Divider, Stack, Typography } from "@mui/material";
import { ReactNode } from "react";

interface PageHeaderProps {
  title: string
  subtitle?: string
  action?: ReactNode
}

export function PageHeader({ title, subtitle, action }: PageHeaderProps) {
  return (
    <Stack spacing={2}>
      <Stack
        direction={{ xs: 'column', sm: 'row' }}
        justifyContent="space-between"
        alignItems={{ xs: 'flex-start', sm: 'flex-end' }}
        spacing={2}
      >
        <Stack spacing={0.5}>
          <Typography variant="h4" sx={{ fontSize: { xs: 26, md: 32 } }}>
            {title}
          </Typography>
          {subtitle && (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              {subtitle}
            </Typography>
          )}
        </Stack>
        {action}
      </Stack>
      <Divider />
    </Stack>
  )
}
