# MAGE: Microframework for Apps on the Google Engine

MAGE is a sessionless, simple web framework built to structure and ease development of web apps running on Google App Engine using the Go API.

It exposes a simple lifecycle and implements its own very performant, low-allocation routing system.

MAGE comes with a fully fledged, optional ORM-like layer (the model package) over the Datastore, Search and Memcache APIs to help keep quotas low while increasing productivity. 

Mage has zero dependencies, except for the Google Client Libraries and the Appengine Package (for obvious reasons).

Mage is 100% compatible with GAE go1.11 API

- Fast and reliable (simple requests are responded to in less than 10000 ns)
- Not invasive: its own packages are all optional, including the router, which means you can build your own system of top of mage
For example, you can mix and match the model package with the official client libraries, or just use the latter.
Mage is built on top of few interfaces: you can use the stock implementations or you can roll your own renderer, use your own router, build your own controller.
- 


### Contents

- [Getting started]
- [The Application interface]
- [Controllers]
- [Handling routes]
- [Authentication]
- [Renderers]
- [The data layer: the model package]



