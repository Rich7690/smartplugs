# Smartplugs

TP-Link has a set of interesting WiFi smart plugs -- the [HS100](http://www.tp-link.com/en/products/details/HS100.html) which provides remote access and control for a power plug, and [HS110](http://www.tp-link.com/en/products/details/cat-5258_HS110.html) which is essentially HS100 but with energy monitoring.

Both smart plugs are only accessible through the Kasa app and there is no official documentation on how to programmatically control either of them. However, they have been reverse engineered and several of these attempts have been documented:

https://www.softscheck.com/en/reverse-engineering-tp-link-hs110/
https://georgovassilis.blogspot.sg/2016/05/controlling-tp-link-hs100-wi-fi-smart.html


This application is packaged as a container you can run via docker, kubernetes, or anything else that accepts these kinds of containers:

`docker pull ghcr.io/rich7690/smartplugs:latest`

Set environment variables to your smart plugs and the application will periodically pull energy data from them and export in prometheus format:

`IP_ADDR=192.168.6.3:192.168.6.2`

Set the port for the web service to listen on:

`PORT=9091` 


Currently the polling happens every 10 seconds in an attempt to not brown out the smart plugs as they seem to be very unreliable. This will probably be configurable soon.

