const COMMONS_API_BASE = "https://commons.wikimedia.org/w/api.php";

interface CommonsSearchCategoriesResponse {
  query: {
    allcategories: { [_: string]: string }[];
  };
}
export interface CategoryOption {
  name: string;
  value: string;
}

export async function searchCategories(term: string) {
  const response = await fetch(
    `${COMMONS_API_BASE}?action=query&format=json&redirects=1&origin=*&list=allcategories&acprefix=${term}`,
  );
  const data = await response.json() as CommonsSearchCategoriesResponse;

  const categoriesList = data.query.allcategories.map(a => a["*"]);
  return categoriesList;
}
