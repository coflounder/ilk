#!/bin/sh
# A fake secret, here to be caught.
CF_ACCESS_CLIENT_ID=fixture.access
CF_ACCESS_CLIENT_SECRET=deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef
curl -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET" "$1"
