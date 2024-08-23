web: sh -c 'pyppeteer-install && gunicorn app:app -k uvicorn.workers.UvicornWorker --workers=1 --timeout 120 --bind 0.0.0.0 --max-requests 5000'
