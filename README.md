VLT: Varnish Load Tester
====

This program takes the output from varnishlog on one server,
and sends identical HTTP requests to another.

* Very easy to use: just 1 argument to specify the target host
* It keeps the same headers as the original requests.
* It lets you run live production traffic on test servers in real time.
* It does not support POST data.

Running
---

    $ ssh varnish-1.dogs.com
    $ vlt varnish-1.staging.dogs.com
    2014/05/28 20:04:52 [301] GET http://www.dogs.com/dogs
    2014/05/28 20:04:53 [200] GET http://www.dogs.com/dogs/
    2014/05/28 20:05:04 [403] GET http://www.dogs.com.au/cats/

Requirements:
* Varnish

Building
---

    $ go build vlt.go

Requirements:
* Go

Compared to varnishreplay
----

Similar results can be had with varnishreplay, which is included with varnish.
However, VLT has the following improvements:
* Easier to use
* Nicer output
* Supports domain names, not just IP4 addresses
