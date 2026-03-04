import { Box, Button, ButtonGroup, Popover, Stack, Typography } from "@mui/material";
import { cloneElement, useState, ReactNode, isValidElement } from "react";

interface PopoverConfirmationButtonProps {
  trigger: ReactNode
  message: ReactNode
  onOk: () => void
  okText: string
  id: string
}

export function PopoverConfirmationButton({ trigger, message, onOk, okText, id }: PopoverConfirmationButtonProps) {
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const open = Boolean(anchorEl);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleOk = () => {
    handleClose()
    onOk()
  }

  return (
    <>
      {isValidElement(trigger) ? cloneElement(trigger, { onClick: handleClick } as React.HTMLAttributes<HTMLElement>) : trigger}
      <Popover
        id={open ? id : undefined}
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
      >
        <Box sx={{ p: 1 }}>
          <Typography sx={{ p: 2 }}>
            {message}
          </Typography>
          <Stack alignItems="flex-end">
            <ButtonGroup>
              <Button onClick={handleClose}>Cancel</Button>
              <Button variant="contained" onClick={handleOk}>{okText}</Button>
            </ButtonGroup>
          </Stack>
        </Box>
      </Popover>
    </>
  )

}
