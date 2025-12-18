import { Autocomplete, Chip, TextField } from "@mui/material";
import { useEffect, useState } from "react";
import { useDebounce } from "use-debounce";
import { searchCategories } from "../request/commons";

interface CategoriesSearchInputProps {
  disabled?: boolean;
  value: string[];
  onChange: (categories: string[]) => void;
}
export function CategoriesSearchInput({ value, onChange, disabled }: CategoriesSearchInputProps) {
  const [search, setSearch] = useState("");
  const [debouncedSearch] = useDebounce(search, 500);
  const [options, setOptions] = useState<string[]>([])
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (debouncedSearch.trim()) {
      setLoading(true)
      searchCategories(debouncedSearch)
        .then(options => {
          setOptions(options)
        })
        .catch(err => {
          console.log("Error searching categories: ", err);
        })
        .finally(() => {
          setLoading(false);
        })

    } else {
      setOptions([])
    }

  }, [debouncedSearch, setOptions, setLoading])

  return (
    <Autocomplete
      disabled={disabled}
      loading={loading}
      multiple
      value={value}
      onChange={(_, newValue) => {
        onChange(newValue);
      }}
      options={options}
      getOptionLabel={(option) => option}
      renderValue={(values, getItemProps) =>
        values.map((option, index) => {
          const { key, ...itemProps } = getItemProps({ index });
          return (
            <Chip
              key={key}
              label={option}
              {...itemProps}
            />
          );
        })
      }
      renderInput={(params) => (
        <TextField {...params} value={search} onChange={(e) => setSearch(e.target.value)} disabled={loading || disabled} />
      )}
      freeSolo
    />
  )
}
