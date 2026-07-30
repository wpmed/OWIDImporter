import { Card, CardContent, Skeleton, Stack } from "@mui/material";

export function TaskCardSkeleton() {
  return (
    <Card>
      <CardContent>
        <Stack spacing={1.5}>
          <Stack direction="row" justifyContent="space-between" alignItems="center">
            <Skeleton variant="text" width="55%" height={24} />
            <Skeleton variant="rounded" width={72} height={22} sx={{ borderRadius: 99 }} />
          </Stack>
          <Skeleton variant="text" width="80%" />
          <Skeleton variant="text" width="65%" />
          <Skeleton variant="text" width="40%" />
        </Stack>
      </CardContent>
    </Card>
  )
}
