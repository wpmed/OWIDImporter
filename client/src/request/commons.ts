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

interface CommonsSearchPageExistsResponse {
  query: {
    pages: {
      [key: string]: {
        pageid: number
        ns: number
        title: string
        missing: string
      }
    }
  }

}
export async function searchPageExists(title: string) {
  const response = await fetch(
    `${COMMONS_API_BASE}?action=query&format=json&redirects=1&origin=*&titles=${title}`,
  );
  const data = await response.json() as CommonsSearchPageExistsResponse;
  let found = false;

  if (Object.keys(data.query.pages).length > 0) {
    Object.entries(data.query.pages).forEach((value) => {
      if (!Object.keys(value[1]).includes("missing")) {
        found = true;
      }
    });
  }

  return found;
}
