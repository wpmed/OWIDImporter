import { FormControl, FormControlLabel, FormLabel, Radio, RadioGroup, Stack, Typography } from "@mui/material";

export interface OverwriteOption {
  value: string
  title: string
  description: string
}

interface OverwriteBehaviourFieldProps {
  options: OverwriteOption[]
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

export function OverwriteBehaviourField({ options, value, onChange, disabled }: OverwriteBehaviourFieldProps) {
  return (
    <FormControl disabled={disabled}>
      <FormLabel sx={{ mb: 1, fontWeight: 600, fontSize: 14, color: 'text.primary' }}>
        If a file with the same name exists
      </FormLabel>
      <RadioGroup value={value} onChange={(e) => onChange(e.target.value)}>
        <Stack spacing={1}>
          {options.map(option => (
            <FormControlLabel
              key={option.value}
              value={option.value}
              control={<Radio size="small" sx={{ pt: 0 }} />}
              sx={{ alignItems: 'flex-start', ml: 0 }}
              label={(
                <Stack spacing={0.25}>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>{option.title}</Typography>
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>{option.description}</Typography>
                </Stack>
              )}
            />
          ))}
        </Stack>
      </RadioGroup>
    </FormControl>
  )
}
