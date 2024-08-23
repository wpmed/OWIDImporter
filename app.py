# -*- coding: utf-8 -*-
#
# OWIDImporter tool. This is based on the toolforge python ASGI
# tutorial by Slavina Stefanova.
#
# Copyright (C) 2023 Brian Wolff and contributors
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by the Free
# Software Foundation, either version 3 of the License, or (at your option)
# any later version.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
# FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
# more details.
#
# You should have received a copy of the GNU General Public License along
# with this program.  If not, see <http://www.gnu.org/licenses/>.

import httpx

from fastapi import FastAPI, Cookie, HTTPException, WebSocket, WebSocketDisconnect, WebSocketException
from fastapi.responses import HTMLResponse, RedirectResponse

import asyncio
from asyncio import TimeoutError
from pyppeteer import launch
import time
import os
import tempfile
import shutil
from typing import Annotated
import uuid
import html
from urllib.parse import urlparse, urlencode
import re
import glob
from hashlib import sha1
import traceback

from dotenv import load_dotenv
load_dotenv()

from requests_oauthlib import OAuth1Session

app = FastAPI()

# TODO: Does this work properly with multiple threads
sessions = {}

DEBUG = os.getenv( 'OWID_DEBUG', '' ) != ''

# HTML of the logged out home page
htmlLogin = """
<!DOCTYPE html>
<html>
	<head><title>OWID importer</title><style>p {font-size: larger}</style></head>
	<body>
	<h1>OWID Importer</h1>
	<p>This is a tool to import freely licensed graphs from OurWorldInData into Wikimedia Commons. To continue, please <a href="/login">login</a>.
	</body>
</html>
"""

# HTML of the logged in Home page
htmlHome = """
<!DOCTYPE html>
<html>
	<head>
		<title>Import OWID chart</title>
		<style>
			.log-log { color: grey }
			.log-success { font-weight: bold; color: darkgreen }
			.log-error { color: red; font-weight: bold }
			.user { float: right }
			.hidden { display: none }
			#owidsrc { border: 1px dashed blue; padding: 1em; margin-left: 4em; margin-right: 4em }
		</style>
	</head>
	<body>
		<div class="user">Hello, <em>$USERNAME</em>.<br>(<a href="/logout">logout</a>)</div>
		<h1>Import OWID chart</h1>
		<p>You can use $NAME (filename without extension), $YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders</p>
		<p>This only works for graphs that are maps with data over multiple years</p>
		<form action="" onsubmit="start(event)">
			<label for="url">URL: </label><input type="url" name="url" id="url" size="80" placeholder="https://ourworldindata.org/grapher/<NAME OF GRAPH>"><br>
			<label for="filename">Filename: </label>
			<input type="text" id="filename" autocomplete="off" value="$NAME,$REGION,$YEAR.svg" size="75">
			<br>
			<label for="description">File description</label>
			<textarea id="description" cols="80" rows="20">
=={{int:filedesc}}==
{{Information
|description={{en|1=$TITLE, $REGION}}
|author = Our World In Data
|date= $YEAR
|source = $URL
|permission = "License: All of Our World in Data is completely open access and all work is licensed under the Creative Commons BY license. You have the permission to use, distribute, and reproduce in any medium, provided the source and authors are credited."
|other versions =
}}
{{Map showing old data|year=$YEAR}}
=={{int:license-header}}==
{{cc-by-4.0}}
[[Category:Our_World_in_Data_maps]]
[[Category:$YEAR maps of {{subst:#ifeq:$REGION|World|the world|$REGION}}]]
[[Category:SVG maps by Our World in Data]]
			</textarea>
			<br>
			<input type="submit" value="Start" id="startbutton">
			<button onclick="doCancel()" type="button" id="cancel" disabled>Cancel</button>
		</form>
		<ul id='messages'>
		</ul>
		<div id="restart" class="hidden"><a href="/">Upload another chart or try again?</a></div>
		<div class="hidden" id="owidsrcdiv">
			<h2>OWID slider gallery source page syntax</h2>
			<p>If using this with {{owidslider}}, you can use the following wikicode for the gallery list page:
			<pre id="owidsrc"></pre>
		<br>
		<small><a href="https://github.com/bawolff/OWIDImporter">source code</a></small>
		<script>
			var ws = null;
			var onmessage = function(info) {
				if ( info.type === 'wikitext' ) {
					document.getElementById( "owidsrc" ).innerText = info.msg
					document.getElementById( "owidsrcdiv" ).className = ''
				} else {
					var messages = document.getElementById('messages')
					var message = document.createElement('li')
					message.className = "log-" + info.type
					var content = document.createTextNode(info.msg)
					message.appendChild(content)
					messages.appendChild(message)
				}
			};
			function start(event) {
				var l = window.location
				ws = new WebSocket(((l.protocol === "https:") ? "wss://" : "ws://") + l.host + "/ws");
				ws.addEventListener( "open", (e) => {
					var url = document.getElementById("url").value
					var filename = document.getElementById("filename").value
					var desc = document.getElementById("description").value
					ws.send(JSON.stringify({action: "start", url: url, filename: filename, desc: desc}))
				} );
				ws.addEventListener( "close", (e) => {
					if ( e.code == '1000' && e.wasClean ) {
						onmessage( { msg: "Connection closed", type: "msg" } );
					} else {
						onmessage( { msg: "Connection error (" + e.code + e.reason + ")", type: "error" } );
					}
					document.getElementById( "restart" ).className = "restart";
				} );
				ws.addEventListener( "message", (event) => {
					var info = JSON.parse( event.data );
					onmessage( info );
				} );
				document.getElementById( 'cancel' ).disabled = false;
				for ( const i of [ "startbutton", "filename", "description", "url" ] ) {
					document.getElementById( i ).disabled = true;
				}
				event.preventDefault()
			}
			function doCancel(e) {
				ws.send(JSON.stringify({ "action": "cancel"}));
				setTimeout(() => ws.close(), 2000);
			}

		</script>
	</body>
</html>
"""

"""Fetch username of current oauth user"""
def getUsername( sessionId ):
	res = doApiReq( sessionId, { 'meta': 'userinfo', 'action': 'query' } )
	if res == None or 'anon' in res['query']['userinfo']:
		return None
	return res['query']['userinfo']['name']

"""Make a MW API request using OAUTH"""
def doApiReq( sessionId, params, files=None ):
	if sessionId == None or sessionId not in sessions or "resource_owner_key" not in sessions[sessionId]:
		return None
	s = sessions[sessionId]
	oauth = OAuth1Session(
		os.getenv( "OWID_OAUTH_TOKEN" ),
		client_secret=os.getenv( "OWID_OAUTH_SECRET" ),
		resource_owner_key=s["resource_owner_key"],
		resource_owner_secret=s["resource_owner_secret"],
		verifier=s['oauth_verifier']
	)
	url = os.getenv( "OWID_MW_API" ) + "?"
	if url == None:
		raise Exception( "No MW API url" )
	if "format" not in params:
		params["format"] = "json"
		params["formatversion"] = 2
	url = url + urlencode(params)
	if files == None:
		return oauth.get( url ).json()
	else:
		return oauth.post( os.getenv( "OWID_MW_API" ), data=params, files=files ).json()

@app.get( "/login" )
async def login():
	initiate_url = os.getenv( "OWID_OAUTH_INITIATE" )
	authorize_url = os.getenv( "OWID_OAUTH_AUTH" )
	secret = os.getenv( "OWID_OAUTH_SECRET" )
	token = os.getenv( "OWID_OAUTH_TOKEN" )
	if initiate_url == None or secret == None or token == None or authorize_url == None:
		raise Exception( "Missing OAUTH vars" )
	oauth = OAuth1Session( token, client_secret=secret )
	reqtokens = oauth.fetch_request_token(initiate_url)

	session_id = str(uuid.uuid4())
	sessions[session_id] = {
		"resource_owner_key_temp": reqtokens.get( 'oauth_token' ),
		"resource_owner_secret_temp": reqtokens.get( "oauth_token_secret" )
	}
	full_auth_url = oauth.authorization_url(authorize_url)
	resp = RedirectResponse( full_auth_url )
	# Maybe consider using __Host- prefix
	resp.set_cookie(
		key="owidsession",
		value=session_id,
		samesite="Lax",
		max_age=60*60*24*7,
		httponly=True,
		secure=False if DEBUG else True
	)
	return resp

@app.get( "/logout" )
async def logout():
	resp = RedirectResponse( "/" )
	resp.delete_cookie( key="owidsession" )
	return resp

#OAuth callback url
@app.get( "/callback" )
async def oauth_callback(
	owidsession: Annotated[str, Cookie()],
	oauth_verifier: str,
	oauth_token: str
):
	if owidsession not in sessions:
		return HTMLResponse( "Invalid session id", status_code=403 )
	s = sessions[owidsession]
	oauth = OAuth1Session(
		os.getenv( "OWID_OAUTH_TOKEN" ),
		client_secret=os.getenv( "OWID_OAUTH_SECRET" ),
		resource_owner_key=s["resource_owner_key_temp"],
		resource_owner_secret=s["resource_owner_secret_temp"],
		verifier=oauth_verifier
	)
	oauth_tokens = oauth.fetch_access_token( os.getenv( "OWID_OAUTH_TOKEN_URL" ) )
	sessions[owidsession]["resource_owner_key"] = oauth_tokens.get('oauth_token')
	sessions[owidsession]["resource_owner_secret"] = oauth_tokens.get('oauth_token_secret')
	sessions[owidsession]["oauth_verifier"] = oauth_verifier
	return RedirectResponse( '/' )


@app.get("/")
async def hello( owidsession: Annotated[str|None, Cookie()] = None ):
	user = getUsername( owidsession )
	if user == None:
		return HTMLResponse( htmlLogin )
	else:
		return HTMLResponse( htmlHome.replace( "$USERNAME", html.escape( user ) ) )


@app.websocket( "/ws" )
async def websocket(websocket: WebSocket, owidsession: Annotated[str, Cookie()]):
	user = getUsername( owidsession )
	origin = urlparse( websocket.headers["Origin"] )
	if origin.netloc != websocket.base_url.netloc:
		raise WebSocketException(code=1008, reason="CSRF")
	# Make sure logged in.
	if user == None:
		raise WebSocketException(code=1008, reason="Not logged in")
	try:
		await websocket.accept()
		data = await websocket.receive_json()
	except WebSocketDisconnect:
		return
	if 'action' in data and data['action'] == 'start':
		await browse(websocket, data, owidsession)
		await websocket.close()
	else:
		await sendMsg( websocket, "error", "Invalid action" )
		await websocket.close(code=1003, reason="Invalid action")

"""Send a message over the websocket"""
async def sendMsg( ws, type, msg ):
	rec = ws.receive_json()
	await ws.send_json( {"type": type, "msg": msg} )
	try:
		# TODO: There is probably a much better way to do this.
		res = await asyncio.wait_for( rec, timeout=0.01 )
		if 'action' in res and res['action'] == 'cancel':
			raise Exception( "User cancelled" )
		else:
			raise Exception( "Unknown message" )
	except TimeoutError:
		pass

def validateParameters( data ):
	if "url" not in data or "filename" not in data or "desc" not in data:
		raise Exception( "Missing information" )
	if not re.match( "^https://ourworldindata.org/grapher/[-a-z_0-9]+(\\?.*)?$", data['url'], re.I ):
		raise Exception( "Invalid url" )

"""Browse to graph, save all SVGs, upload them"""
async def browse(ws, data, sessionId):
	items = {}
	browser = None
	try:
		validateParameters( data )
		pagename = data['url']
		await sendMsg( ws, "msg", "Starting" )
		browser = await launch({"headless": True})
		page = await browser.newPage()
		# downloadPathParent = os.path.dirname(os.path.abspath( __file__ )) + '/downloads'
		downloadPathParent = tempfile.gettempdir()
		await page.setUserAgent( os.getenv( 'OWID_UA', 'Medwiki OWIDExporter' ) )
		await page.goto( pagename, { "waitUntil": 'load' } )
		await sendMsg( ws, "msg", "Loaded " + pagename )
		# In rare cases, it seems like we get here before fully loaded despite waiting for load event.
		for i in range( 30 ):
			loaded = await page.evaluate( '() => document.querySelector( ".timeline-component .startMarker") !== null ' )
			if loaded == True:
				break
			else:
				await sendMsg( ws, "log", "Waiting for browser... " )
				time.sleep(1)
		else:
			raise Exception( "Page never finished loading. Please retry" )

		years = {}
		# Download an SVG for every year on the graph
		for y in range( 2 if DEBUG else 300 ):
			currentYear = await page.evaluate( '() => document.querySelector( ".timeline-component .startMarker").getAttribute( "aria-valuenow" ) ' )
			if currentYear in years:
				break;
			years[currentYear] = True;
			regions = {}
			# Download an SVG for each world region
			for x in range( 20 ):
				currentRegion = await page.evaluate( '() => document.querySelector( ".map-projection-menu .control" ).textContent' )
				if currentRegion in regions:
					# we did all regions, restart on next year.
					break
				regions[currentRegion] = True
				currentTitle = await page.evaluate( '() => document.querySelector( "figure h1" ).textContent' ) 
				downloadPath = tempfile.mkdtemp( dir=downloadPathParent )
				#print(downloadPath + " " + currentRegion + " " + currentYear)
				items[downloadPath] = { 'region': currentRegion, 'year': currentYear, 'title': currentTitle }
				await saveSVG( downloadPath, page )
				await sendMsg( ws, "log", "Downloaded " + currentRegion + " " + currentYear )
				# Go to next region. hacky
				await page.focus( '#react-select-2-input' )
				await page.keyboard.press( ' ' )
				await page.keyboard.press( 'ArrowDown' )
				await page.keyboard.press( 'Enter' )
			else:
				raise Exception( "Loop detected in region switch" )
			await page.focus( '.timeline-component .startMarker' )
			await page.keyboard.press( 'ArrowLeft' )
		else:
			if not DEBUG:
				raise exception( "Loop detected in year switch or > 300 years" )
		# Make sure all downloads are finished before closing the browser.
		time.sleep(3)
		await browser.close()
		browser = None
		await sendMsg( ws, "msg", "Finished downloading SVGs" )
		await uploadFiles( data, items, sessionId, ws )
		await sendTemplate( items, ws )
		return items
	except Exception as e:
		try:
			details = ' ' + str(traceback.format_exc()) if DEBUG else ''
			await sendMsg( ws, "error", "Error: " + str(e) + details )
		except:
			pass
	finally:
		# it should be fine if browser is closed multiple times.
		if browser != None:
			await browser.close()
		for i in items:
			shutil.rmtree( i, ignore_errors=True )

"""Download an SVG file into the given directory"""
async def saveSVG( downloadPath, page ):
	await page._client.send( 'Page.setDownloadBehavior', { 'behavior': 'allow', 'downloadPath': downloadPath } )
	await page.evaluate( '() => document.querySelector( \'button[aria-label="Download"]\').click()' )
	for x in range(100):
		ready = await page.evaluate( '() => document.querySelector( \'button[data-track-note="chart_download_svg"]\') !== null' )
		if ready:
			await page.evaluate( '() => document.querySelector( \'button[data-track-note="chart_download_svg"]\').click()' )
			await page.evaluate( '() => document.querySelector( \'button.close-button\').click()' )
			return
		else:
			time.sleep( 0.25 )
	raise Exception( "Could not save svg" )

"""Take the files and actually upload them"""
async def uploadFiles( params, items, sessionId, ws ):
	await sendMsg( ws, 'debug', 'Fetching edit token' )
	token = doApiReq( sessionId, { 'action': 'query', 'meta': 'tokens' } )['query']['tokens']['csrftoken']
	uploaded = 0
	failures = 0
	overwrote = 0
	skipped = 0
	for filedirectory in items:
		item = items[filedirectory]
		await sendMsg( ws, "log", "Processing " + item['region'] + ', ' + item['year'] + ' for upload' )	
		fileInfo = getFileInfo(filedirectory)
		filedesc = replaceVars( params['desc'], params, item, fileInfo )
		filename = replaceVars( params['filename'], params, item, fileInfo )
		items[filedirectory]["uploadName"] = filename

		res = doApiReq( sessionId, { 'action': 'query', 'prop': 'imageinfo', 'titles': 'File:' + filename, 'iiprop': 'sha1' } )
		page = res['query']['pages'][0]
		if 'imageinfo' not in page:
			# do upload
			res = doApiReq( sessionId, {
				'action': 'upload',
				'comment': 'Importing from ' + params['url'],
				'text': filedesc,
				'filename': filename,
				'ignorewarnings': '1',
				'token': token
			}, { 'file': ( filename, fileInfo['file'], 'image/svg+xml' ) } )
			if 'upload' in res and 'result' in res['upload'] and res['upload']['result'] == 'Success':
				await sendMsg( ws, "msg", "Succesfully uploaded File:" + filename )
				uploaded += 1
			else:
				await sendMsg( ws, "error", "Error uploading File:" + filename + "\n" + str(res) )
				failures += 1
		elif page['imageinfo'][0]['sha1'] == fileInfo['sha1']:
			# Already uploaded.
			await sendMsg( ws, "log", "Skipping File:" + filename + " since exact duplicate of existing file" )
			skipped += 1
		else:
			# Overwrite
			res = doApiReq( sessionId, {
				'action': 'upload',
				'comment': 'Re-importing from ' + params['url'],
				# Should be ignored, but keep just in case
				'text': filedesc,
				'filename': filename,
				'ignorewarnings': '1',
				'token': token
			}, { 'file': ( filename, fileInfo['file'], 'image/svg+xml' ) } )
			if 'upload' in res and 'result' in res['upload'] and res['upload']['result'] == 'Success':
				await sendMsg( ws, "msg", "Succesfully overwrote File:" + filename )
				overwrote += 1
			else:
				await sendMsg( ws, "error", "Error uploading File:" + filename + "\n" + str(res) )
				failures += 1
	if failures == 0:
		await sendMsg( ws, "success", "All files uploaded. {uploaded} new files, {overwrote} re-uploads, {skipped} skipped".format( uploaded=uploaded, overwrote=overwrote, skipped=skipped ) )
	else:
		await sendMsg( ws, "error", "Some files could not be uploaded. {uploaded} new files, {overwrote} re-uploads, {skipped} skipped, {failed} failed".format( uploaded=uploaded, overwrote=overwrote, skipped=skipped, failed=failures ) )

"""Replace variables in file description and on wiki file name"""
def replaceVars( value, params, item, fileInfo ):
	value = value.replace( '$URL', params['url'] )
	value = value.replace( '$NAME', fileInfo['name'] )
	value = value.replace( '$TITLE', item['title'] )
	value = value.replace( '$YEAR', item['year'] )
	value = value.replace( '$REGION', item['region'] )
	return value

"""Given a directory with 1 svg in it, get the modified file contents, name and sha1"""
def getFileInfo(filedirectory):
	files = glob.glob( filedirectory + "/*.svg" )
	results = {}
	if len(files) != 1:
		raise Exception( "Found too many files " + str(files) )
	with open(files[0], 'rb') as fh:
		filecontents = fh.read()
	# Commons does not allow external fonts
	results['file'] = re.sub( b"<style>@impo[^<]*</style>", b"", filecontents )
	results['name'] = re.sub( "\\.svg$", "", files[0] )
	results['name'] = re.sub( "^.*/", "", results['name'] )
	results['sha1'] = sha1( results['file'] ).hexdigest()
	return results

"""Send the wikitext syntax to make a {{owidslidersrcs}} gallery"""
async def sendTemplate( items, ws ):
	galleries = {}
	wikitext = '{{owidslidersrcs|id=gallery|widths=640|heights=640\n'
	for dir in items:
		item = items[dir]
		if item['region'] not in galleries:
			galleries[item['region']] = []
		galleries[item['region']].append( item )
	for region in galleries:
		galleries[region].sort( key = lambda x: x['year'] )
		wikitext += '|gallery-' + region + '=\n'
		for i in galleries[region]:
			wikitext += 'File:' + i['uploadName'] + '!year=' + i['year'] + '\n'
	wikitext += '}}\n'
	await sendMsg( ws, 'wikitext', wikitext )


