# Transmission Proxy

Application intended to be reverse proxy in front of Transmission daemon
and able to filter requests to implement safety restrictions. In default configuration it:

* restricts all location-related settings (default download dir, individual torrents' locations)
  to the prefix specified in `DOWNLOAD_PREFIX`,
* disallows RPC method `torrent-rename-path`,
* disallows (skips) fields `incomplete-dir*`, `peer-port*`, `script-torrent*` from settings update requests.

The app implements whitelist on methods and their arguments, so in case updated Transmission client
offers new methods or new arguments for old methods they will not be available until they will be deemed safe to use.

## Transmission access control (authentication etc.)

This app transfers all request headers to transmission, so all authentication details
should work as if you spoke to the Transmission daemon directly. The app provides
no additional security so it is the user's responsibility to protect their instance of Transmission
and this app from an-authorized use.

## Configuration

All configuration is done via setting corresponding environment var:

* `DOWNLOAD_PREFIX` (required, e.g. `/downloads/`),
* `UPSTREAM_HOST` (require, e.g. `http://127.0.0.1:9091`),
* `DEBUG_MODE`(optional, set to `yes`/`on`/`true` to return errors in response). Unless you are debugging
  this application and would like to see the error messages in HTTP responses, do not set this variable
  and instead only error IDs will be provided in responses while full error messages will be available in logs.
