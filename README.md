# OWIDImporter

This is a simple tool to import freely licensed OWID graphs into Wikimedia Commons.
Wikimedia Commons does not allow web fonts in SVGs, so the files are modified slightly.

The website for this tool is https://owidimporter.toolforge.org

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.


### Installing

First, clone the repository to your local machine:

```bash
git clone https://github.com/bawolff/OWIDImporter
cd OWIDImporter
```

Install the required packages:

```bash
pip install -r requirements.txt
pyppeteer-install
```

## Running the Application

You can start the development server by running:

```bash
uvicorn app:app --reload
```

The application will start running at <http://127.0.0.1:8000/>

The API Docs will be available at <http://127.0.0.1:8000/docs>

Note: The app does not work with multiple threads as of right now.

## Running in production on toolforge

To configure, use `toolforge envvars create <environmental variable>`

To update, run: `toolforge build start https://github.com/bawolff/OWIDImporter && toolforge webservice restart`

For info on how to build for production, see https://wikitech.wikimedia.org/wiki/Help:Toolforge/Build_Service

## Config
You need to create a .env file with the appropriate values. See env.example for an example.

You also need a MW install that you have OAuth setup with.

## License

This project is licensed under the GNU General Public License v3.0 - see the LICENSE file for details.
